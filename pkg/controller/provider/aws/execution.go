// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	dnserrors "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
)

type Change struct {
	*route53types.Change
	Done        provider.DoneHandler
	UpdateGroup string
}

// changeRecordsAPI is the narrow subset of the Route53 client used by Execution.
// It exists so the batching logic (submitChanges/tryFixChanges/submitBisected)
// can be exercised with a fake in tests. *route53.Client satisfies it.
type changeRecordsAPI interface {
	ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
	ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
}

type Execution struct {
	logger.LogContext
	metrics       provider.Metrics
	r53           changeRecordsAPI
	policyContext *routingPolicyContext
	rateLimiter   flowcontrol.RateLimiter
	zone          provider.DNSHostedZone

	changes   map[dns.DNSSetName][]*Change
	batchSize int
}

func NewExecution(logger logger.LogContext, h *Handler, zone provider.DNSHostedZone) *Execution {
	return &Execution{
		LogContext:    logger,
		metrics:       h.config.Metrics,
		r53:           &h.r53,
		policyContext: h.policyContext,
		rateLimiter:   h.config.RateLimiter,
		zone:          zone,
		changes:       map[dns.DNSSetName][]*Change{},
		batchSize:     h.awsConfig.BatchSize,
	}
}

func buildResourceRecordSet(ctx context.Context, name dns.DNSSetName, policy *dns.RoutingPolicy, policyContext *routingPolicyContext, rset *dns.RecordSet) (*route53types.ResourceRecordSet, error) {
	if rrs, err := buildResourceRecordSetForAliasTarget(ctx, name, policy, policyContext, rset); rrs != nil || err != nil {
		return rrs, err
	}
	rrs := &route53types.ResourceRecordSet{}
	rrs.Name = aws.String(name.DNSName)
	rrs.Type = route53types.RRType(rset.Type)
	rrs.TTL = aws.Int64(rset.TTL)
	rrs.ResourceRecords = make([]route53types.ResourceRecord, len(rset.Records))
	for i, r := range rset.Records {
		rrs.ResourceRecords[i] = route53types.ResourceRecord{
			Value: aws.String(r.Value),
		}
	}
	if err := policyContext.addRoutingPolicy(ctx, rrs, name, policy); err != nil {
		return nil, err
	}
	return rrs, nil
}

func (this *Execution) addChange(ctx context.Context, action route53types.ChangeAction, req *provider.ChangeRequest, dnsset *dns.DNSSet) error {
	name := dnsset.Name.Align()
	rset := dnsset.Sets[req.Type]
	if len(rset.Records) == 0 {
		return nil
	}
	this.Infof("%s %s record set %s[%s]: %s(%d)", action, rset.Type, name, this.zone.Id(), rset.RecordString(), rset.TTL)

	var policy *dns.RoutingPolicy
	if req.Addition != nil {
		policy = req.Addition.RoutingPolicy
	} else if req.Deletion != nil {
		policy = req.Deletion.RoutingPolicy
	}
	rrs, err := buildResourceRecordSet(ctx, name, policy, this.policyContext, rset)
	if err != nil {
		this.Errorf("addChange failed for %s[%s]: %s", name, this.zone.Id(), err)
		return err
	}

	change := &route53types.Change{Action: action, ResourceRecordSet: rrs}
	this.addRawChange(name, dnsset.UpdateGroup, change, req.Done)

	return nil
}

func (this *Execution) addRawChange(name dns.DNSSetName, updateGroup string, change *route53types.Change, done provider.DoneHandler) {
	this.changes[name] = append(this.changes[name], &Change{Change: change, Done: done, UpdateGroup: updateGroup})
}

func (this *Execution) submitChanges(ctx context.Context) error {
	if len(this.changes) == 0 {
		return nil
	}

	failed := 0
	throttlingErrCount := 0
	successCount := 0
	limitedChanges := limitChangeSet(this.changes, this.batchSize)
	this.Infof("require %d batches for %d dns names", len(limitedChanges), len(this.changes))
	for i, changes := range limitedChanges {
		this.Infof("processing batch %d for zone %s with %d requests", i+1, this.zone.Id(), len(changes))
		for _, c := range changes {
			extraInfo := ""
			if c.ResourceRecordSet.AliasTarget != nil {
				extraInfo = fmt.Sprintf(" (alias target hosted zone %s)", *c.ResourceRecordSet.AliasTarget.HostedZoneId)
			}
			this.Infof("desired change: %s %s %s%s", c.Action, *c.ResourceRecordSet.Name, c.ResourceRecordSet.Type, extraInfo)
		}

		params := &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(this.zone.Id().ID),
			ChangeBatch: &route53types.ChangeBatch{
				Changes: mapChanges(changes),
			},
		}

		this.metrics.AddZoneRequests(this.zone.Id().ID, provider.M_UPDATERECORDS, 1)
		this.rateLimiter.Accept()
		var succeededChanges, failedChanges []*Change
		_, err := this.r53.ChangeResourceRecordSets(ctx, params)
		if err != nil {
			failedChanges = changes
			handled := false
			throttling := false
			var apiError smithy.APIError
			if errors.As(err, &apiError) {
				switch v := apiError.(type) {
				case *route53types.InvalidChangeBatch:
					succeededChanges, failedChanges, err = this.tryFixChanges(ctx, v.ErrorMessage(), changes)
					handled = true // tryFixChanges already bisects its unclear remainder
				default:
					if v.ErrorCode() == "Throttling" {
						throttling = true
						throttlingErrCount++
					}
				}
			}
			// For non-throttling errors that were not already handled as an InvalidChangeBatch,
			// bisect the batch so a single bad change does not fail its valid batch-mates.
			// Throttling errors are left as a whole-batch failure to be retried later;
			// bisecting them would not help and would only multiply the load.
			if !handled && !throttling && len(failedChanges) > 1 {
				var additionalSucceededChanges []*Change
				additionalSucceededChanges, failedChanges, err = this.submitBisected(ctx, failedChanges)
				succeededChanges = append(succeededChanges, additionalSucceededChanges...)
			}
		} else {
			succeededChanges = changes
		}
		if len(failedChanges) > 0 {
			for _, c := range failedChanges {
				failed++
				if c.Done != nil {
					c.Done.Failed(stableError(err))
				}
			}
			this.Errorf("%d records in zone %s fail: %s", len(failedChanges), this.zone.Id(), err)
		}
		if len(succeededChanges) > 0 {
			successCount += len(succeededChanges)
			for _, c := range succeededChanges {
				if c.Done != nil {
					c.Done.Succeeded()
				}
			}
			this.Infof("%d records in zone %s were successfully updated", len(succeededChanges), this.zone.Id())
		}
	}
	if failed > 0 {
		err := fmt.Errorf("%d changes failed", failed)
		if throttlingErrCount == len(limitedChanges) {
			err = dnserrors.NewThrottlingError(err)
		}
		if successCount == 0 {
			return dnserrors.NewAllChangesFailedError(err)
		}
		return err
	}
	return nil
}

var (
	patternNotFound = regexp.MustCompile(`Tried to delete resource record set \[name='([^']+)', type='([^']+)'] but it was not found`)
	patternExists   = regexp.MustCompile(`Tried to create resource record set \[name='([^']+)', type='([^']+)'] but it already exists`)
)

func (this *Execution) tryFixChanges(ctx context.Context, message string, changes []*Change) (succeeded []*Change, failed []*Change, err error) {
	submatchNotFound := patternNotFound.FindAllStringSubmatch(message, -1)
	submatchExists := patternExists.FindAllStringSubmatch(message, -1)
	var unclear []*Change
outer:
	for _, change := range changes {
		switch change.Action {
		case route53types.ChangeActionDelete:
			for _, m := range submatchNotFound {
				if dns.NormalizeHostname(m[1]) == dns.NormalizeHostname(*change.ResourceRecordSet.Name) && m[2] == string(change.ResourceRecordSet.Type) {
					this.Infof("Ignoring already deleted record: %s (%s)",
						*change.ResourceRecordSet.Name, change.ResourceRecordSet.Type)
					succeeded = append(succeeded, change)
					continue outer
				}
			}
		case route53types.ChangeActionCreate:
			for _, m := range submatchExists {
				if dns.NormalizeHostname(m[1]) == dns.NormalizeHostname(*change.ResourceRecordSet.Name) && m[2] == string(change.ResourceRecordSet.Type) {
					if this.isFetchedRecordSetEqual(ctx, change) {
						this.Infof("Ignoring already created record: %s (%s)",
							*change.ResourceRecordSet.Name, change.ResourceRecordSet.Type)
						succeeded = append(succeeded, change)
						continue outer
					}
				}
			}
		}
		unclear = append(unclear, change)
	}

	if len(unclear) > 0 {
		s, f, ferr := this.submitBisected(ctx, unclear)
		succeeded = append(succeeded, s...)
		failed = append(failed, f...)
		if ferr != nil {
			err = ferr
		}
	}
	return
}

// submitBisected submits the given changes as one batch. On rejection it splits
// the batch in half and retries each half recursively, so a single bad change
// only fails itself instead of its valid batch-mates. It returns the changes
// that were successfully applied, the ones that could not be applied, and the
// error from the smallest failing batch (nil if all changes succeeded).
func (this *Execution) submitBisected(ctx context.Context, changes []*Change) (succeeded, failed []*Change, err error) {
	if len(changes) == 0 {
		return nil, nil, nil
	}

	params := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(this.zone.Id().ID),
		ChangeBatch: &route53types.ChangeBatch{
			Changes: mapChanges(changes),
		},
	}
	this.metrics.AddZoneRequests(this.zone.Id().ID, provider.M_UPDATERECORDS, 1)
	this.rateLimiter.Accept()
	if _, callErr := this.r53.ChangeResourceRecordSets(ctx, params); callErr == nil {
		return changes, nil, nil
	} else if len(changes) == 1 {
		// A single change still fails: it is the bad one.
		return nil, changes, callErr
	}

	mid := len(changes) / 2
	s1, f1, err1 := this.submitBisected(ctx, changes[:mid])
	s2, f2, err2 := this.submitBisected(ctx, changes[mid:])
	// Build fresh slices: s1/f1 may alias the backing array of changes[:mid],
	// so append(s1, s2...) could overwrite changes[mid] (and thus corrupt f2).
	succeeded = append(succeeded, s1...)
	succeeded = append(succeeded, s2...)
	failed = append(failed, f1...)
	failed = append(failed, f2...)
	// Prefer a leaf (single-change) error so the message names a real cause.
	err = err1
	if err == nil {
		err = err2
	}
	return
}

func (this *Execution) isFetchedRecordSetEqual(ctx context.Context, change *Change) bool {
	output, err := this.r53.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:          aws.String(this.zone.Id().ID),
		MaxItems:              aws.Int32(1),
		StartRecordIdentifier: nil,
		StartRecordName:       change.ResourceRecordSet.Name,
		StartRecordType:       change.ResourceRecordSet.Type,
	})
	if err != nil || len(output.ResourceRecordSets) == 0 {
		return false
	}
	crrs := change.ResourceRecordSet
	orrs := output.ResourceRecordSets[0]
	if dns.NormalizeHostname(*crrs.Name) != dns.NormalizeHostname(*orrs.Name) || crrs.Type != orrs.Type || !safeCompareInt64(crrs.TTL, orrs.TTL) || len(crrs.ResourceRecords) != len(orrs.ResourceRecords) {
		return false
	}
	for i := range crrs.ResourceRecords {
		if *crrs.ResourceRecords[i].Value != *orrs.ResourceRecords[i].Value {
			return false
		}
	}
	return true
}

func safeCompareInt64(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func limitChangeSet(changesByName map[dns.DNSSetName][]*Change, max int) [][]*Change {
	batches := [][]*Change{}

	updateChanges := map[string][]*Change{}
	// add deletion requests
	batch := make([]*Change, 0)
	for _, changes := range changesByName {
		for _, change := range changes {
			if change.Action == route53types.ChangeActionDelete {
				batch, batches = addLimited(change, batch, batches, max)
			} else {
				arr := updateChanges[change.UpdateGroup]
				arr = append(arr, change)
				updateChanges[change.UpdateGroup] = arr
			}
		}
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
		batch = make([]*Change, 0)
	}

	// add non-deletion requests
	for _, changes := range updateChanges {
		for _, change := range changes {
			batch, batches = addLimited(change, batch, batches, max)
		}
		// new batch for every update group
		batches = append(batches, batch)
		batch = make([]*Change, 0)
	}

	return batches
}

func addLimited(change *Change, batch []*Change, batches [][]*Change, max int) ([]*Change, [][]*Change) {
	if len(batch) >= max {
		batches = append(batches, batch)
		batch = make([]*Change, 0)
	}
	return append(batch, change), batches
}

func mapChanges(changes []*Change) []route53types.Change {
	dest := []route53types.Change{}
	for _, c := range changes {
		dest = append(dest, *c.Change)
	}
	return dest
}

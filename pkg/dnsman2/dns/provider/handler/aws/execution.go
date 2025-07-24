// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go"
	"github.com/go-logr/logr"
	"k8s.io/client-go/util/flowcontrol"

	dnserrors "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

type wrappedChange struct {
	*route53types.Change
}

type execution struct {
	log           logr.Logger
	r53           route53.Client
	policyContext *routingPolicyContext
	rateLimiter   flowcontrol.RateLimiter
	zoneID        dns.ZoneID

	changes   []*wrappedChange
	batchSize int
}

func newExecution(log logr.Logger, h *handler, zoneID dns.ZoneID) *execution {
	return &execution{
		log:           log,
		r53:           h.r53,
		policyContext: h.policyContext,
		rateLimiter:   h.config.RateLimiter,
		zoneID:        zoneID,
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
		value := r.Value
		if rrs.Type == route53types.RRTypeTxt {
			value = strconv.Quote(value)
		}
		rrs.ResourceRecords[i] = route53types.ResourceRecord{
			Value: aws.String(value),
		}
	}
	if err := policyContext.addRoutingPolicy(ctx, rrs, name, policy); err != nil {
		return nil, err
	}
	return rrs, nil
}

func (ex *execution) addChange(ctx context.Context, action route53types.ChangeAction, reqs provider.ChangeRequests, rs *dns.RecordSet) error {
	name := reqs.Name.EnsureTrailingDot()

	ex.log.Info(fmt.Sprintf("%s %s record set %s: %s(%d)", action, rs.Type, name, rs.RecordString(), rs.TTL), "zoneID", ex.zoneID)

	rrs, err := buildResourceRecordSet(ctx, name, rs.RoutingPolicy, ex.policyContext, rs)
	if err != nil {
		ex.log.Error(err, "addChange failed", "name", name, "zoneID", ex.zoneID)
		return err
	}

	change := &route53types.Change{Action: action, ResourceRecordSet: rrs}
	ex.changes = append(ex.changes, &wrappedChange{Change: change})

	return nil
}

func (ex *execution) submitChanges(ctx context.Context, metrics provider.Metrics) error {
	if len(ex.changes) == 0 {
		return nil
	}

	failed := 0
	throttlingErrCount := 0
	limitedChanges := limitChangeSet(ex.changes, ex.batchSize)
	ex.log.Info("batches required", "batches", len(limitedChanges), "totalChangeCount", len(ex.changes))
	for i, changes := range limitedChanges {
		ex.log.Info("processing batch", "batch", i+1, "zoneID", ex.zoneID, "changeCount", len(changes))
		for _, c := range changes {
			extraInfo := ""
			if c.ResourceRecordSet.AliasTarget != nil {
				extraInfo = fmt.Sprintf(" (alias target hosted zone %s)", *c.ResourceRecordSet.AliasTarget.HostedZoneId)
			}
			ex.log.Info(fmt.Sprintf("desired change: %s %s %s%s", c.Action, *c.ResourceRecordSet.Name, c.ResourceRecordSet.Type, extraInfo))
		}

		params := &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(ex.zoneID.ID),
			ChangeBatch: &route53types.ChangeBatch{
				Changes: mapChanges(changes),
			},
		}

		metrics.AddZoneRequests(ex.zoneID.ID, provider.MetricsRequestTypeUpdateRecords, 1)
		ex.rateLimiter.Accept()
		var succeededChanges, failedChanges []*wrappedChange
		_, err := ex.r53.ChangeResourceRecordSets(ctx, params)
		if err != nil {
			failedChanges = changes
			var apiError smithy.APIError
			if errors.As(err, &apiError) {
				switch v := apiError.(type) {
				case *route53types.InvalidChangeBatch:
					succeededChanges, failedChanges, err = ex.tryFixChanges(ctx, v.ErrorMessage(), changes)
				default:
					if v.ErrorCode() == "Throttling" {
						throttlingErrCount++
					}
				}
			}
		} else {
			succeededChanges = changes
		}
		if len(failedChanges) > 0 {
			failed += len(failedChanges)
			ex.log.Error(err, fmt.Sprintf("%d records failed", len(failedChanges)), "zoneID", ex.zoneID)
		}
		if len(succeededChanges) > 0 {
			ex.log.Info(fmt.Sprintf("%d records were successfully updated", len(succeededChanges)), "zoneID", ex.zoneID)
		}
	}
	if failed > 0 {
		err := fmt.Errorf("%d changes failed", failed)
		if throttlingErrCount == len(limitedChanges) {
			err = dnserrors.NewThrottlingError(err)
		}
		return err
	}
	return nil
}

var (
	patternNotFound = regexp.MustCompile(`Tried to delete resource record set \[name='([^']+)', type='([^']+)'] but it was not found`)
	patternExists   = regexp.MustCompile(`Tried to create resource record set \[name='([^']+)', type='([^']+)'] but it already exists`)
)

func (ex *execution) tryFixChanges(ctx context.Context, message string, changes []*wrappedChange) (succeeded []*wrappedChange, failed []*wrappedChange, err error) {
	submatchNotFound := patternNotFound.FindAllStringSubmatch(message, -1)
	submatchExists := patternExists.FindAllStringSubmatch(message, -1)
	var unclear []*wrappedChange
outer:
	for _, change := range changes {
		switch change.Action {
		case route53types.ChangeActionDelete:
			for _, m := range submatchNotFound {
				if dns.NormalizeDomainName(m[1]) == dns.NormalizeDomainName(*change.ResourceRecordSet.Name) && m[2] == string(change.ResourceRecordSet.Type) {
					ex.log.Info("ignoring already deleted record", "record",
						fmt.Sprintf("%s (%s)", *change.ResourceRecordSet.Name, change.ResourceRecordSet.Type))
					succeeded = append(succeeded, change)
					continue outer
				}
			}
		case route53types.ChangeActionCreate:
			for _, m := range submatchExists {
				if dns.NormalizeDomainName(m[1]) == dns.NormalizeDomainName(*change.ResourceRecordSet.Name) && m[2] == string(change.ResourceRecordSet.Type) {
					if ex.isFetchedRecordSetEqual(ctx, change) {
						ex.log.Info("ignoring already created record: %s (%s)", "record",
							fmt.Sprintf("%s (%s)", *change.ResourceRecordSet.Name, change.ResourceRecordSet.Type))
						succeeded = append(succeeded, change)
						continue outer
					}
				}
			}
		}
		unclear = append(unclear, change)
	}

	if len(unclear) > 0 {
		params := &route53.ChangeResourceRecordSetsInput{
			HostedZoneId: aws.String(ex.zoneID.ID),
			ChangeBatch: &route53types.ChangeBatch{
				Changes: mapChanges(unclear),
			},
		}
		_, err = ex.r53.ChangeResourceRecordSets(ctx, params)
		if err != nil {
			failed = append(failed, unclear...)
		} else {
			succeeded = append(succeeded, unclear...)
		}
	}
	return
}

func (ex *execution) isFetchedRecordSetEqual(ctx context.Context, change *wrappedChange) bool {
	output, err := ex.r53.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:          aws.String(ex.zoneID.ID),
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
	if dns.NormalizeDomainName(*crrs.Name) != dns.NormalizeDomainName(*orrs.Name) || crrs.Type != orrs.Type || !safeCompareInt64(crrs.TTL, orrs.TTL) || len(crrs.ResourceRecords) != len(orrs.ResourceRecords) {
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

func limitChangeSet(changes []*wrappedChange, max int) [][]*wrappedChange {
	var batches [][]*wrappedChange

	// add deletion requests
	batch := make([]*wrappedChange, 0)
	for _, change := range changes {
		if change.Action == route53types.ChangeActionDelete {
			batch, batches = addLimited(change, batch, batches, max)
		}
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
		batch = make([]*wrappedChange, 0)
	}

	// add non-deletion requests
	for _, change := range changes {
		if change.Action != route53types.ChangeActionDelete {
			batch, batches = addLimited(change, batch, batches, max)
		}
	}

	if len(batch) > 0 {
		batches = append(batches, batch)
	}

	return batches
}

func addLimited(change *wrappedChange, batch []*wrappedChange, batches [][]*wrappedChange, max int) ([]*wrappedChange, [][]*wrappedChange) {
	if len(batch) >= max {
		batches = append(batches, batch)
		batch = make([]*wrappedChange, 0)
	}
	return append(batch, change), batches
}

func mapChanges(changes []*wrappedChange) []route53types.Change {
	var dest []route53types.Change
	for _, c := range changes {
		dest = append(dest, *c.Change)
	}
	return dest
}

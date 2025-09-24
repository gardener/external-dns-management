// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package raw

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/go-logr/logr"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

// Executor is the interface that wraps the basic DNS record operations.
type Executor interface {
	// CreateRecord creates a new DNS record in the given hosted zone.
	CreateRecord(ctx context.Context, r Record, zone provider.DNSHostedZone) error
	// UpdateRecord updates an existing DNS record in the given hosted zone.
	UpdateRecord(ctx context.Context, r Record, zone provider.DNSHostedZone) error
	// DeleteRecord deletes an existing DNS record in the given hosted zone.
	DeleteRecord(ctx context.Context, r Record, zone provider.DNSHostedZone) error

	// NewRecord creates a new Record instance for the given parameters.
	// The returned Record must be a new instance and not a reference to an existing one.
	NewRecord(fqdn, rtype, value string, zone provider.DNSHostedZone, ttl int64) Record
	// GetRecordList returns the list of records for the given DNS name and record type in the given hosted zone.
	// It also returns a list of routing policies corresponding to the records, or nil if the provider does not support routing policies.
	// The length of the returned record list and routing policy list must be the same if routing policies are supported.
	// If no records are found, an empty list and nil are returned.
	GetRecordList(ctx context.Context, dnsName, rtype string, zone provider.DNSHostedZone) (RecordList, []*dns.RoutingPolicy, error)
}

// Execution manages the execution of DNS record changes for a specific DNS name in a specific hosted zone.
type Execution struct {
	log      logr.Logger
	executor Executor
	zone     provider.DNSHostedZone
	domain   string
	name     dns.DNSSetName

	routingPolicyChecker RoutingPolicyChecker

	additions RecordList
	updates   RecordList
	deletions RecordList

	results map[dns.DNSSetName]error
}

// RoutingPolicyChecker is a function that checks if the routing policy in the change request is supported.
type RoutingPolicyChecker func(name dns.DNSSetName, req *provider.ChangeRequestUpdate) error

func defaultRoutingPolicyChecker(name dns.DNSSetName, req *provider.ChangeRequestUpdate) error {
	if name.SetIdentifier != "" || (req.New != nil && req.New.RoutingPolicy != nil) || (req.Old != nil && req.Old.RoutingPolicy != nil) {
		return fmt.Errorf("routing policy not supported")
	}
	return nil
}

// NewExecution creates a new Execution instance for the given parameters.
func NewExecution(log logr.Logger, executor Executor, zone provider.DNSHostedZone, name dns.DNSSetName, checker RoutingPolicyChecker) *Execution {
	if checker == nil {
		checker = defaultRoutingPolicyChecker
	}
	return &Execution{
		log:                  log,
		executor:             executor,
		zone:                 zone,
		name:                 name,
		domain:               zone.Domain(),
		routingPolicyChecker: checker,
		results:              map[dns.DNSSetName]error{},
		additions:            RecordList{},
		updates:              RecordList{},
		deletions:            RecordList{},
	}
}

// AddChange adds a change request to the execution.
// It classifies the change as an addition, update, or deletion and prepares it for submission.
func (exec *Execution) AddChange(ctx context.Context, req *provider.ChangeRequestUpdate) error {
	if req.New.Length() == 0 && req.Old.Length() == 0 {
		return nil
	}

	err := exec.routingPolicyChecker(exec.name, req)
	if err != nil {
		exec.log.Info(fmt.Sprintf("warning: record set %s[%s]: %s", exec.name, exec.zone.ZoneID().ID, err))
		return err
	}

	beforeCount := len(exec.additions) + len(exec.updates) + len(exec.deletions)
	switch {
	case req.Old == nil && req.New != nil:
		exec.log.Info(fmt.Sprintf("create %s record set %s[%s]: %s(%d)", req.New.Type, exec.name, exec.zone.ZoneID().ID, req.New.RecordString(), req.New.TTL))
		if err := exec.add(ctx, req.New, true, &exec.updates, &exec.additions, &exec.deletions); err != nil {
			return err
		}
	case req.Old != nil && req.New != nil:
		exec.log.Info(fmt.Sprintf("update %s record set %s[%s]: %s(%d)", req.New.Type, exec.name, exec.zone.ZoneID().ID, req.New.RecordString(), req.New.TTL))
		if err := exec.add(ctx, req.New, true, &exec.updates, &exec.additions, &exec.deletions); err != nil {
			return err
		}
	case req.Old != nil && req.New == nil:
		exec.log.Info(fmt.Sprintf("delete %s record set %s[%s]: %s", req.Old.Type, exec.name, exec.zone.ZoneID().ID, req.Old.RecordString()))
		if err := exec.add(ctx, req.Old, false, &exec.deletions, nil, &exec.deletions); err != nil {
			return err
		}
	}
	afterCount := len(exec.additions) + len(exec.updates) + len(exec.deletions)
	if afterCount == beforeCount {
		exec.log.Info("no changes required")
	}
	return nil
}

func (exec *Execution) add(ctx context.Context, rs *dns.RecordSet, modonly bool, found *RecordList, notfound *RecordList, diffList *RecordList) error {
	oldRL, oldRoutingPolicies, err := exec.executor.GetRecordList(ctx, exec.name.DNSName, string(rs.Type), exec.zone)
	if err != nil {
		return err
	}
	oldFound := make([]bool, len(oldRL))
	for _, r := range rs.Records {
		value := r.Value
		if rs.Type == dns.TypeTXT {
			value = strconv.Quote(value)
		}
		var (
			old              Record
			oldRoutingPolicy *dns.RoutingPolicy
		)
		for i, or := range oldRL {
			if oldFound[i] {
				continue
			}
			if or.GetValue() == value {
				old = or
				if oldRoutingPolicies != nil {
					oldRoutingPolicy = oldRoutingPolicies[i]
				}
				oldFound[i] = true
				break
			}
		}
		if old != nil {
			if (!modonly) || (old.GetTTL() != rs.TTL) || !reflect.DeepEqual(oldRoutingPolicy, rs.RoutingPolicy) {
				or := old.Clone()
				or.SetTTL(rs.TTL)
				or.SetRoutingPolicy(exec.name.SetIdentifier, rs.RoutingPolicy)
				*found = append(*found, or)
			}
		} else {
			if notfound != nil {
				record := exec.executor.NewRecord(exec.name.DNSName, string(rs.Type), r.Value, exec.zone, rs.TTL)
				record.SetRoutingPolicy(exec.name.SetIdentifier, rs.RoutingPolicy)
				*notfound = append(*notfound, record)
			}
		}
	}
	if diffList != nil {
	outer:
		for i, or := range oldRL {
			if !oldFound[i] {
				if or.GetSetIdentifier() == exec.name.SetIdentifier {
					for _, r := range *diffList {
						if r.GetId() == or.GetId() {
							continue outer
						}
					}
					*diffList = append(*diffList, or)
				}
			}
		}
	}
	return nil
}

// SubmitChanges submits all accumulated changes to the DNS provider.
// It returns an error if any of the changes fail.
func (exec *Execution) SubmitChanges(ctx context.Context) error {
	if len(exec.additions) == 0 && len(exec.updates) == 0 && len(exec.deletions) == 0 {
		return nil
	}

	exec.log.Info("processing changes", "zone", exec.zone.ZoneID().ID)
	for _, r := range exec.additions {
		exec.log.Info(fmt.Sprintf("desired change: Addition %s %s: %s (%d)", r.GetDNSName(), r.GetType(), r.GetValue(), r.GetTTL()))
		exec.submit(ctx, exec.executor.CreateRecord, r)
	}
	for _, r := range exec.updates {
		exec.log.Info(fmt.Sprintf("desired change: Update %s %s: %s (%d)", r.GetDNSName(), r.GetType(), r.GetValue(), r.GetTTL()))
		exec.submit(ctx, exec.executor.UpdateRecord, r)
	}
	for _, r := range exec.deletions {
		exec.log.Info(fmt.Sprintf("desired change: Deletion %s %s: %s", r.GetDNSName(), r.GetType(), r.GetValue()))
		exec.submit(ctx, exec.executor.DeleteRecord, r)
	}

	failed := 0
	succeeded := 0
	for _, err := range exec.results {
		if err != nil {
			failed++
		} else {
			succeeded++
		}
	}

	if succeeded > 0 {
		exec.log.Info("Succeeded updates for records", "zone", exec.zone.ZoneID().ID, "count", succeeded)
	}
	if failed > 0 {
		exec.log.Info("Failed updates for records", "zone", exec.zone.ZoneID().ID, "count", failed)
		return fmt.Errorf("could not update all dns entries")
	}
	return nil
}

func (exec *Execution) submit(ctx context.Context, f func(ctx context.Context, record Record, zone provider.DNSHostedZone) error, r Record) {
	err := f(ctx, r, exec.zone)
	if err != nil {
		exec.log.Error(err, "execution failed", "recordType", r.GetType(), "name", r.GetDNSName())
	}
	exec.results[dns.DNSSetName{DNSName: r.GetDNSName(), SetIdentifier: r.GetSetIdentifier()}] = err
}

// ExecuteRequests executes the given change requests in the specified hosted zone using the provided executor.
// It returns an error if any of the changes fail.
func ExecuteRequests(
	ctx context.Context,
	log logr.Logger,
	executor Executor,
	zone provider.DNSHostedZone,
	reqs provider.ChangeRequests,
	checker RoutingPolicyChecker,
) error {
	exec := NewExecution(log, executor, zone, reqs.Name, checker)
	for _, r := range reqs.Updates {
		if err := exec.AddChange(ctx, r); err != nil {
			return err
		}
	}
	return exec.SubmitChanges(ctx)
}

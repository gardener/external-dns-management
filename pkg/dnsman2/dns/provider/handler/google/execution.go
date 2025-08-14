// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package google

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/go-logr/logr"
	googledns "google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

const (
	googleRecordTTL = 300
)

type execAction int

const (
	createAction execAction = 1
	deleteAction execAction = -1
)

type execution struct {
	log     logr.Logger
	handler *handler
	zoneID  dns.ZoneID
	change  *googledns.Change

	routingPolicyChanges routingPolicyChanges
}

func newExecution(log logr.Logger, h *handler, zoneID dns.ZoneID) *execution {
	change := &googledns.Change{
		Additions: []*googledns.ResourceRecordSet{},
		Deletions: []*googledns.ResourceRecordSet{},
	}
	ex := &execution{
		log:                  log,
		handler:              h,
		zoneID:               zoneID,
		change:               change,
		routingPolicyChanges: routingPolicyChanges{},
	}
	return ex
}

func (ex *execution) addChange(action execAction, reqs provider.ChangeRequests, rs *dns.RecordSet) error {
	policy, err := extractRoutingPolicy(reqs.Name, rs)
	if err != nil {
		return err
	}

	setName := reqs.Name.EnsureTrailingDot()
	switch action {
	case createAction:
		ex.log.Info(fmt.Sprintf("create %s record set %s[%s]: %s(%d)", rs.Type, setName, ex.zoneID, rs.RecordString(), rs.TTL))
		ex.addAddition(mapRecordSet(setName, rs, policy))
	case deleteAction:
		ex.log.Info(fmt.Sprintf("delete %s record set %s[%s]: %s", rs.Type, setName, ex.zoneID, rs.RecordString()))
		ex.addDeletion(mapRecordSet(setName, rs, policy))
	default:
		return fmt.Errorf("unknown action %d for record set %s[%s]: %s", action, setName, ex.zoneID, rs.RecordString())
	}
	return nil
}

func (ex *execution) addAddition(set *googledns.ResourceRecordSet) {
	if set.RoutingPolicy == nil {
		ex.change.Additions = append(ex.change.Additions, set)
		return
	}

	ex.routingPolicyChanges.addChange(set, true)
}

func (ex *execution) addDeletion(set *googledns.ResourceRecordSet) {
	if set.RoutingPolicy == nil {
		ex.change.Deletions = append(ex.change.Deletions, set)
		return
	}

	ex.routingPolicyChanges.addChange(set, false)
}

func (ex *execution) prepareSubmission(rrsetGetter rrsetGetterFunc) error {
	routingPolicyDeletions, routingPolicyAdditions, err := ex.routingPolicyChanges.calcDeletionsAndAdditions(rrsetGetter)
	if err != nil {
		return err
	}

	for _, c := range ex.change.Deletions {
		ex.log.Info(fmt.Sprintf("desired change: Deletion %s %s: %s", c.Name, c.Type, utils.Strings(c.Rrdatas...)))
	}
	for _, c := range routingPolicyDeletions {
		ex.log.Info(fmt.Sprintf("desired change: Deletion %s %s (routing policy: %s)", c.Name, c.Type, describeRoutingPolicy(c)))
		ex.change.Deletions = append(ex.change.Deletions, c)
	}
	for _, c := range ex.change.Additions {
		ex.log.Info(fmt.Sprintf("desired change: Addition %s %s: %s", c.Name, c.Type, utils.Strings(c.Rrdatas...)))
	}
	for _, c := range routingPolicyAdditions {
		ex.log.Info(fmt.Sprintf("desired change: Addition %s %s (routing policy: %s)", c.Name, c.Type, describeRoutingPolicy(c)))
		ex.change.Additions = append(ex.change.Additions, c)
	}
	return nil
}

func (ex *execution) submitChanges(metrics provider.Metrics) error {
	if len(ex.change.Additions) == 0 && len(ex.change.Deletions) == 0 && len(ex.routingPolicyChanges) == 0 {
		return nil
	}

	ex.log.Info("processing changes", "zone", ex.zoneID)
	projectID, zoneName, err := splitZoneID(ex.zoneID.ID)
	if err != nil {
		ex.log.Error(err, "failed to split zone ID", "zoneID", ex.zoneID.ID)
		return err
	}
	rrsetGetter := func(name string, typ dns.RecordType) (*googledns.ResourceRecordSet, error) {
		return ex.handler.getResourceRecordSet(projectID, zoneName, name, string(typ))
	}
	if err := ex.prepareSubmission(rrsetGetter); err != nil {
		ex.log.Error(err, "failed to prepare submission", "zoneID", ex.zoneID)
		return err
	}

	metrics.AddZoneRequests(ex.zoneID.ID, provider.MetricsRequestTypeUpdateRecords, 1)
	ex.handler.config.RateLimiter.Accept()
	if _, err := ex.handler.service.Changes.Create(projectID, zoneName, ex.change).Do(); err != nil {
		if apiErr, ok := err.(*googleapi.Error); ok && apiErr.Code == 412 {
			// Check if the order of the records is the problem
			// Background: The record order for A and AAAA records is not guaranteed for DNS queries.
			// Most DNS servers, including authoritative ones, may randomize or rotate the order for load balancing or other reasons.
			err = ex.retryDeletionWithActualRecords(err, projectID, zoneName)
		}
		if err != nil {
			ex.log.Error(err, "failed to submit changes", "zoneID", ex.zoneID)
			return err
		}
	}

	ex.log.Info(fmt.Sprintf("%d records added and %d deleted in zone %s", len(ex.change.Additions), len(ex.change.Deletions), ex.zoneID))
	return nil
}

func (ex *execution) retryDeletionWithActualRecords(oldErr error, projectID, zoneName string) error {
	if len(ex.change.Deletions) == 0 {
		// no deletions, nothing to retry
		return oldErr
	}

	var newDeletions []*googledns.ResourceRecordSet
	for _, rs := range ex.change.Deletions {
		realRS, err := ex.handler.getResourceRecordSet(projectID, zoneName, rs.Name, rs.Type)
		if err != nil {
			if isNotFound(err) {
				// record set does not exist, already deleted
				continue
			}
			return errors.Join(oldErr, fmt.Errorf("failed to get record set %s[%s] for reordering: %w", rs.Name, rs.Type, err))
		}
		mismatchMessage := compareRecordSets(rs, realRS)
		if mismatchMessage != "" {
			ex.log.Info(fmt.Sprintf("warning: record set %s[%s] does not match expected deletion: %s", rs.Name, rs.Type, mismatchMessage))
		}
		newDeletions = append(newDeletions, realRS)
	}
	ex.change.Deletions = newDeletions
	_, err := ex.handler.service.Changes.Create(projectID, zoneName, ex.change).Do()
	return err
}

func isNotFound(err error) bool {
	if ge, ok := err.(*googleapi.Error); ok {
		return ge.Code == 404
	}
	return false
}

func compareRecordSets(expected, actual *googledns.ResourceRecordSet) string {
	var msgs []string
	if expected.Ttl != actual.Ttl {
		msgs = append(msgs, fmt.Sprintf("TTL mismatch: expected %d, got %d", expected.Ttl, actual.Ttl))
	}
	expectedSet := sets.NewString(expected.Rrdatas...)
	actualSet := sets.NewString(actual.Rrdatas...)
	extraInExpected := expectedSet.Difference(actualSet)
	if extraInExpected.Len() > 0 {
		msgs = append(msgs, fmt.Sprintf("extra rrdatas in expected: %v", extraInExpected.List()))
	}
	extraInActual := actualSet.Difference(expectedSet)
	if extraInActual.Len() > 0 {
		msgs = append(msgs, fmt.Sprintf("extra rrdatas in actual: %v", extraInActual.List()))
	}
	if extraInExpected.Len() == 0 && extraInActual.Len() == 0 {
		msgs = append(msgs, "rrdatas match, but order may differ")
	}
	return strings.Join(msgs, "; ")
}

func mapRecordSet(name dns.DNSSetName, rs *dns.RecordSet, policy *googleRoutingPolicyData) *googledns.ResourceRecordSet {
	targets := make([]string, len(rs.Records))
	for i, r := range rs.Records {
		switch rs.Type {
		case dns.TypeCNAME:
			targets[i] = dns.EnsureTrailingDot(r.Value)
		case dns.TypeTXT:
			targets[i] = strconv.Quote(r.Value)
		default:
			targets[i] = r.Value
		}
	}

	// no annotation results in a TTL of 0, default to 300 for backwards-compatibility
	var ttl int64 = googleRecordTTL
	if rs.TTL > 0 {
		ttl = rs.TTL
	}

	rrset := &googledns.ResourceRecordSet{
		Name:    name.DNSName,
		Rrdatas: targets,
		Ttl:     ttl,
		Type:    string(rs.Type),
	}
	rrset = mapPolicyRecordSet(rrset, policy)
	return rrset
}

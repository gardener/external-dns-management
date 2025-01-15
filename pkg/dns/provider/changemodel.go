// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

////////////////////////////////////////////////////////////////////////////////
// Requests
////////////////////////////////////////////////////////////////////////////////

const (
	R_CREATE = "create"
	R_UPDATE = "update"
	R_DELETE = "delete"
)

type ChangeRequests []*ChangeRequest

type ChangeRequest struct {
	Action   string
	Type     string
	Addition *dns.DNSSet
	Deletion *dns.DNSSet
	Done     DoneHandler
	Applied  bool
}

func (r *ChangeRequest) IsSemanticEqualTo(other *ChangeRequest) bool {
	return r.Action == other.Action && r.Type == other.Type &&
		(r.Addition == nil && other.Addition == nil || r.Addition != nil && other.Addition != nil && r.Addition.MatchRecordTypeSubset(other.Addition, r.Type)) &&
		(r.Deletion == nil && other.Deletion == nil || r.Deletion != nil && other.Deletion != nil && r.Deletion.MatchRecordTypeSubset(other.Deletion, r.Type))
}

func NewChangeRequest(action string, rtype string, del, add *dns.DNSSet, done DoneHandler) *ChangeRequest {
	r := &ChangeRequest{Action: action, Type: rtype, Addition: add, Deletion: del}
	r.Done = &applyingDoneHandler{changeRequest: r, inner: done}
	return r
}

type applyingDoneHandler struct {
	changeRequest *ChangeRequest
	inner         DoneHandler
}

func (h *applyingDoneHandler) SetInvalid(err error) {
	if h.inner != nil {
		h.inner.SetInvalid(err)
	}
}

func (h *applyingDoneHandler) Failed(err error) {
	if h.inner != nil {
		h.inner.Failed(err)
	}
}

func (h *applyingDoneHandler) Throttled() {
	if h.inner != nil {
		h.inner.Throttled()
	}
}

func (h *applyingDoneHandler) Succeeded() {
	h.changeRequest.Applied = true
	if h.inner != nil {
		h.inner.Succeeded()
	}
}

type ChangeGroup struct {
	name          string
	provider      DNSProvider
	dnssets       dns.DNSSets
	requests      ChangeRequests
	model         *ChangeModel
	providerCount int

	cleanedMetadataRecords int
}

func newChangeGroup(name string, provider DNSProvider, model *ChangeModel) *ChangeGroup {
	return &ChangeGroup{name: name, provider: provider, dnssets: dns.DNSSets{}, requests: ChangeRequests{}, model: model}
}

func (this *ChangeGroup) cleanup(logger logger.LogContext, model *ChangeModel) bool {
	mod := false
	for _, s := range this.dnssets {
		_, ok := model.applied[s.Name]
		if !ok {
			if model.ExistsInEquivalentZone(s.Name) {
				continue
			}
			if e := model.IsStale(ZonedDNSSetName{ZoneID: model.ZoneId(), DNSSetName: s.Name}); e != nil {
				if e.IsDeleting() {
					model.failedDNSNames.Add(s.Name) // preventing deletion of stale entry
				}
				status := e.Object().Status()
				msg := MSG_PRESERVED
				trigger := false
				if status.State == api.STATE_ERROR || status.State == api.STATE_INVALID {
					msg = msg + ": " + utils.StringValue(status.Message)
					model.Infof("found stale set '%s': %s -> preserve unchanged", utils.StringValue(status.Message), s.Name)
				} else {
					model.Infof("found stale set '%s' -> preserve unchanged", s.Name)
					trigger = true
				}
				upd, err := e.UpdateStatus(logger, api.STATE_STALE, msg)
				if trigger && (!upd || err != nil) {
					e.Trigger(logger)
				}
			} else {
				oldSet := model.oldDNSSets[s.Name]
				if oldSet == nil {
					// not part of transaction, but old metadata entries may be present for cleanup
					mod = this.partialCleanupOfMetadataRecords(logger, s) || mod
					continue
				}
				model.Infof("found unapplied managed set '%s'", s.Name)
				var done DoneHandler
				for _, e := range model.context.entries {
					if e.dnsSetName == s.Name {
						done = NewStatusUpdate(logger, e, model.context.fhandler)
						break
					}
				}
				for ty := range s.Sets {
					if _, ok := oldSet.Sets[ty]; !ok {
						continue
					}
					mod = true
					this.addDeleteRequest(s, ty, model.wrappedDoneHandler(s.Name, done))
				}
			}
		}
	}
	return mod
}

func (this *ChangeGroup) partialCleanupOfMetadataRecords(logger logger.LogContext, s *dns.DNSSet) bool {
	if this.cleanedMetadataRecords >= this.model.config.MaxMetadataRecordDeletionsPerReconciliation {
		// Maximum number of metadata records to delete per reconciliation reached.
		// To avoid excessive deletions, we stop here and continue in the next reconciliation
		return false
	}
	if this.model.ownership == nil || len(this.model.ownership.GetIds()) == 0 {
		// no known owners to clean up metadata records
		return false
	}

	if set, ok := s.Sets[dns.RS_TXT]; ok {
		name := s.Name.DNSName
		for _, prefix := range []string{"comment-", "*.comment-"} {
			if strings.HasPrefix(name, prefix) {
				var foundPrefix, foundOwner bool
				for _, r := range set.Records {
					v, _ := strconv.Unquote(r.Value)
					if strings.HasPrefix(v, "prefix=comment-") {
						foundPrefix = true
					} else if strings.HasPrefix(v, "owner=") && this.model.ownership.IsResponsibleFor(strings.TrimPrefix(v, "owner=")) {
						foundOwner = true
					}
				}
				if foundPrefix && foundOwner {
					logger.Infof("cleaning up metadata record for %s", name)
					this.cleanedMetadataRecords++
					this.addDeleteRequest(s, dns.RS_TXT, nil)
					return true
				}
			}
		}
	}
	return false
}

func (this *ChangeGroup) update(logger logger.LogContext, model *ChangeModel) bool {
	ok := true
	model.Infof("reconcile entries for %s (with %d requests)", this.name, len(this.requests))

	reqs := this.requests
	if len(reqs) > 0 {
		this.model.context.dnsTicker.TickWhile(logger, func() {
			err := this.provider.ExecuteRequests(logger, model.context.zone.getZone(), this.model.zonestate, reqs)
			if err != nil {
				model.Errorf("entry reconciliation failed for %s: %s", this.name, err)
				ok = false
			}
		})
	}
	return ok
}

func (this *ChangeGroup) addCreateRequest(dnsset *dns.DNSSet, rtype string, done DoneHandler) {
	this.addChangeRequest(R_CREATE, nil, dnsset, rtype, done)
}

func (this *ChangeGroup) addUpdateRequest(old, new *dns.DNSSet, rtype string, done DoneHandler) {
	this.addChangeRequest(R_UPDATE, old, new, rtype, done)
}

func (this *ChangeGroup) addDeleteRequest(dnsset *dns.DNSSet, rtype string, done DoneHandler) {
	this.addChangeRequest(R_DELETE, dnsset, nil, rtype, done)
}

func (this *ChangeGroup) addChangeRequest(action string, old, new *dns.DNSSet, rtype string, done DoneHandler) {
	r := NewChangeRequest(action, rtype, old, new, done)
	this.requests = append(this.requests, r)
}

type TargetSpec = dnsutils.TargetSpec

////////////////////////////////////////////////////////////////////////////////
// Change Model
////////////////////////////////////////////////////////////////////////////////

type ChangeModel struct {
	logger.LogContext
	config         Config
	ownership      dns.Ownership
	context        *zoneReconciliation
	applied        map[dns.DNSSetName]*dns.DNSSet
	dangling       *ChangeGroup
	providergroups map[string]*ChangeGroup
	zonestate      DNSZoneState
	failedDNSNames dns.DNSNameSet
	oldDNSSets     dns.DNSSets
}

type ChangeResult struct {
	Modified bool
	Retry    bool
	Error    error
}

func NewChangeModel(logger logger.LogContext, ownership dns.Ownership, req *zoneReconciliation, config Config, oldDNSSets dns.DNSSets) *ChangeModel {
	return &ChangeModel{
		LogContext:     logger,
		config:         config,
		ownership:      ownership,
		context:        req,
		applied:        map[dns.DNSSetName]*dns.DNSSet{},
		providergroups: map[string]*ChangeGroup{},
		failedDNSNames: dns.DNSNameSet{},
		oldDNSSets:     oldDNSSets,
	}
}

func (this *ChangeModel) IsStale(dns ZonedDNSSetName) *Entry {
	return this.context.stale[dns]
}

func (this *ChangeModel) ExistsInEquivalentZone(name dns.DNSSetName) bool {
	return this.context.equivEntries != nil && this.context.equivEntries.Contains(name)
}

func (this *ChangeModel) getProviderView(p DNSProvider) *ChangeGroup {
	v := this.providergroups[p.AccountHash()]
	if v == nil {
		name := fmt.Sprintf("%s[%s]", p.ObjectName().String(), atMost(p.AccountHash(), 8))
		v = newChangeGroup(name, p, this)
		this.providergroups[p.AccountHash()] = v
	}
	v.providerCount++
	return v
}

func (this *ChangeModel) ZoneId() dns.ZoneID {
	return this.context.zone.Id()
}

func (this *ChangeModel) Domain() string {
	return this.context.zone.Domain()
}

// getDefaultProvider returns a provider of the change group with the most providers.
func (this *ChangeModel) getDefaultProvider() DNSProvider {
	var oldest DNSProvider
	for _, p := range this.context.providers {
		if oldest == nil || oldest.Object().GetCreationTimestamp().Time.After(p.Object().GetCreationTimestamp().Time) {
			oldest = p
		}
	}
	return oldest
}

func (this *ChangeModel) dumpf(fmt string, args ...interface{}) {
	this.Debugf(fmt, args...)
}

func (this *ChangeModel) Setup() error {
	var err error

	provider := this.getDefaultProvider()
	if provider == nil {
		return fmt.Errorf("no provider found for zone %q", this.ZoneId())
	}
	this.context.dnsTicker.TickWhile(this, func() {
		this.zonestate, err = provider.GetZoneState(this.context.zone.getZone())
	})
	if err != nil {
		return err
	}
	sets := this.zonestate.GetDNSSets()
	this.dangling = newChangeGroup("dangling entries", provider, this)
	for setName, set := range sets {
		var view *ChangeGroup
		provider = this.context.providers.LookupFor(setName.DNSName)
		if provider != nil {
			this.dumpf("  %s: %d types (provider %s)", setName, len(set.Sets), provider.ObjectName())
			view = this.getProviderView(provider)
		} else {
			this.dumpf("  %s: %d types (no provider)", setName, len(set.Sets))
			view = this.dangling
		}
		view.dnssets[setName] = set
		for t, r := range set.Sets {
			this.dumpf("    %s: %d records: %s", t, len(r.Records), r.RecordString())
		}
	}
	this.Infof("found %d entries in zone %s (using %d groups)", len(sets), this.ZoneId(), len(this.providergroups))
	return err
}

func (this *ChangeModel) Check(name dns.DNSSetName, updateGroup string, done DoneHandler, spec TargetSpec) ChangeResult {
	return this.Exec(false, false, name, updateGroup, done, spec)
}

func (this *ChangeModel) Apply(name dns.DNSSetName, updateGroup string, done DoneHandler, spec TargetSpec) ChangeResult {
	return this.Exec(true, false, name, updateGroup, done, spec)
}

func (this *ChangeModel) Delete(name dns.DNSSetName, updateGroup string, done DoneHandler, spec TargetSpec) ChangeResult {
	return this.Exec(true, true, name, updateGroup, done, spec)
}

func (this *ChangeModel) PseudoApply(name dns.DNSSetName, spec TargetSpec) {
	this.applied[name] = dns.NewDNSSet(name, spec.RoutingPolicy())
}

func (this *ChangeModel) Exec(apply bool, delete bool, name dns.DNSSetName, updateGroup string, done DoneHandler, spec TargetSpec) ChangeResult {
	// this.Infof("%s: %v", name, targets)
	if len(spec.Targets()) == 0 && !delete {
		return ChangeResult{}
	}

	if apply {
		this.applied[name] = nil
		done = this.wrappedDoneHandler(name, done)
	}
	p := this.context.providers.LookupFor(name.DNSName)
	if p == nil {
		err := fmt.Errorf("no provider found for %q", name)
		if done != nil {
			if apply {
				done.SetInvalid(err)
			}
		} else {
			this.Warnf("no done handler and %s", err)
		}
		return ChangeResult{Error: err}
	}

	view := this.getProviderView(p)
	oldset := view.dnssets[name]
	newset := dns.NewDNSSet(name, spec.RoutingPolicy())
	newset.UpdateGroup = updateGroup
	if !delete {
		this.ApplySpec(newset, p, spec)
	}
	mod := false
	if oldset != nil {
		this.Debugf("found old for entry %q", oldset.Name)
		for recordType, recordSet := range newset.Sets {
			curset := oldset.Sets[recordType]
			if curset == nil {
				if apply {
					view.addCreateRequest(newset, recordType, done)
				}
				mod = true
			} else {
				olddns := oldset.Name
				newdns := newset.Name
				if olddns == newdns {
					if !curset.Match(recordSet) || !reflect.DeepEqual(spec.RoutingPolicy(), oldset.RoutingPolicy) {
						if apply {
							view.addUpdateRequest(oldset, newset, recordType, done)
						}
						mod = true
					} else {
						if apply {
							this.Debugf("records type %s up to date for %s", recordType, name)
						}
					}
				} else {
					if apply {
						view.addCreateRequest(newset, recordType, done)
						view.addDeleteRequest(oldset, recordType, this.wrappedDoneHandler(name, nil))
					}
					mod = true
				}
			}
		}
		for ty := range oldset.Sets {
			if _, ok := newset.Sets[ty]; !ok {
				if apply {
					view.addDeleteRequest(oldset, ty, done)
				}
				mod = true
			}
		}
	} else {
		if !delete {
			if apply {
				this.Infof("no existing entry found for %s", name)
				for ty := range newset.Sets {
					view.addCreateRequest(newset, ty, done)
				}
			}
			mod = true
		}
	}
	if apply {
		this.applied[name] = newset
		if !mod && done != nil {
			done.Succeeded()
		}
	}
	return ChangeResult{Modified: mod}
}

func (this *ChangeModel) Cleanup(logger logger.LogContext) bool {
	mod := false
	for _, view := range this.providergroups {
		mod = view.cleanup(logger, this) || mod
	}
	mod = this.dangling.cleanup(logger, this) || mod
	if mod {
		logger.Infof("found entries to be deleted")
	}
	return mod
}

func (this *ChangeModel) Update(logger logger.LogContext) error {
	failed := false
	for _, view := range this.providergroups {
		failed = !view.update(logger, this) || failed
	}
	failed = !this.dangling.update(logger, this) || failed
	if failed {
		return fmt.Errorf("entry reconciliation failed for some provider(s)")
	}
	return nil
}

func (this *ChangeModel) IsFailed(name dns.DNSSetName) bool {
	return this.failedDNSNames.Contains(name)
}

func (this *ChangeModel) wrappedDoneHandler(name dns.DNSSetName, done DoneHandler) DoneHandler {
	return &changeModelDoneHandler{
		changeModel: this,
		inner:       done,
		dnsSetName:  name,
	}
}

/////////////////////////////////////////////////////////////////////////////////
// changeModelDoneHandler

type changeModelDoneHandler struct {
	changeModel *ChangeModel
	inner       DoneHandler
	dnsSetName  dns.DNSSetName
}

func (this *changeModelDoneHandler) SetInvalid(err error) {
	if this.inner != nil {
		this.inner.SetInvalid(err)
	}
}

func (this *changeModelDoneHandler) Failed(err error) {
	this.changeModel.failedDNSNames.Add(this.dnsSetName)
	if this.inner != nil {
		this.inner.Failed(err)
	}
}

func (this *changeModelDoneHandler) Succeeded() {
	if this.inner != nil {
		this.inner.Succeeded()
	}
}

func (this *changeModelDoneHandler) Throttled() {
	if this.inner != nil {
		this.inner.Throttled()
	}
}

/////////////////////////////////////////////////////////////////////////////////
// DNSSets

func (this *ChangeModel) ApplySpec(set *dns.DNSSet, provider DNSProvider, spec TargetSpec) *dns.DNSSet {
	targets := provider.MapTargets(set.Name.DNSName, spec.Targets())
	for _, t := range targets {
		set.Sets.AddRecord(t.GetRecordType(), t.GetHostName(), t.GetTTL())
	}
	return set
}

func atMost(s string, maxlen int) string {
	if len(s) < maxlen {
		return s
	}
	return s[:maxlen]
}

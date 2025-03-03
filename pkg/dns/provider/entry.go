// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/access"
	"github.com/gardener/controller-manager-library/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/statistic"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

const MSG_PRESERVED = "errorneous entry preserved in provider"

// maxCNAMETargets is the maximum number of CNAME targets. It is restricted, as it needs regular DNS lookups.
const maxCNAMETargets = 25

type EntryPremise struct {
	ptype    string
	provider DNSProvider
	fallback DNSProvider // provider with correct zone, but outside selection (only set if provider == nil)
	zoneid   string

	// non-identifying fields
	zonedomain string
}

func (this *EntryPremise) Match(p *EntryPremise) bool {
	return this.ptype == p.ptype && this.provider == p.provider && this.zoneid == p.zoneid && this.fallback == p.fallback
}

func (this *EntryPremise) NotifyChange(p *EntryPremise) string {
	r := []string{}
	if this.ptype != p.ptype {
		r = append(r, fmt.Sprintf("provider type (%s -> %s)", this.ptype, p.ptype))
	}
	if this.provider != p.provider {
		r = append(r, fmt.Sprintf("provider (%s -> %s)", Provider(this.provider), Provider(p.provider)))
	}
	if this.zoneid != p.zoneid {
		r = append(r, fmt.Sprintf("zone (%s -> %s)", this.zoneid, p.zoneid))
	}
	if this.fallback != p.fallback {
		r = append(r, fmt.Sprintf("fallback (%s -> %s)", Provider(this.fallback), Provider(p.fallback)))
	}
	if len(r) == 0 {
		return ""
	}
	return "premise changed: " + strings.Join(r, ", ")
}

type EntryVersion struct {
	object        *dnsutils.DNSEntryObject
	providername  resources.ObjectName
	dnsSetName    dns.DNSSetName
	targets       Targets
	routingPolicy *dns.RoutingPolicy
	mappings      map[string][]string
	warnings      []string

	status api.DNSEntryStatus

	interval    int64
	responsible bool
	valid       bool
	duplicate   bool
	obsolete    bool
}

func NewEntryVersion(object *dnsutils.DNSEntryObject, old *Entry) *EntryVersion {
	v := &EntryVersion{
		object:     object,
		dnsSetName: dns.DNSSetName{DNSName: object.GetDNSName(), SetIdentifier: object.GetSetIdentifier()},
		targets:    Targets{},
		mappings:   map[string][]string{},
	}
	if old != nil {
		v.status = old.status
	} else {
		v.status = *object.Status()
	}
	return v
}

func (this *EntryVersion) GetAnnotations() map[string]string {
	return this.object.GetAnnotations()
}

func (this *EntryVersion) RequiresUpdateFor(e *EntryVersion) (reasons []string) {
	if this.dnsSetName != e.dnsSetName {
		reasons = append(reasons, "recordset name changed")
	}
	if !utils.Int64Equal(this.status.TTL, e.status.TTL) {
		reasons = append(reasons, "ttl changed")
	}
	if this.valid != e.valid {
		reasons = append(reasons, "validation state changed")
	}
	if this.ZoneId() != e.ZoneId() {
		reasons = append(reasons, "zone changed")
	}
	if this.targets.DifferFrom(e.targets) {
		reasons = append(reasons, "targets changed")
	}
	if !reflect.DeepEqual(this.routingPolicy, e.routingPolicy) {
		reasons = append(reasons, "routing policy changed")
	}
	if this.State() != e.State() {
		if e.State() != api.STATE_READY {
			reasons = append(reasons, "state changed")
		}
	}
	if this.obsolete != e.obsolete {
		reasons = append(reasons, "provider responsibility changed")
	}
	return
}

func (this *EntryVersion) IsValid() bool {
	return this.valid
}

func (this *EntryVersion) KeepRecords() bool {
	return this.IsValid() || this.status.State != api.STATE_INVALID
}

func (this *EntryVersion) IsDeleting() bool {
	return this.object.IsDeleting()
}

func (this *EntryVersion) Object() *dnsutils.DNSEntryObject {
	return this.object
}

func (this *EntryVersion) Message() string {
	return utils.StringValue(this.status.Message)
}

func (this *EntryVersion) ZoneId() dns.ZoneID {
	var zoneid dns.ZoneID
	if this.status.ProviderType != nil && this.status.Zone != nil {
		zoneid = dns.NewZoneID(*this.status.ProviderType, *this.status.Zone)
	}
	return zoneid
}

func (this *EntryVersion) State() string {
	return this.status.State
}

func (this *EntryVersion) ClusterKey() resources.ClusterObjectKey {
	return this.object.ClusterKey()
}

func (this *EntryVersion) ObjectName() resources.ObjectName {
	return this.object.ObjectName()
}

func (this *EntryVersion) ObjectKey() client.ObjectKey {
	return client.ObjectKey{Namespace: this.object.GetNamespace(), Name: this.object.GetName()}
}

func (this *EntryVersion) DNSName() string {
	return this.dnsSetName.DNSName
}

func (this *EntryVersion) GetSetIdentifier() string {
	return this.dnsSetName.SetIdentifier
}

func (this *EntryVersion) DNSSetName() dns.DNSSetName {
	return this.dnsSetName
}

func (this *EntryVersion) ZonedDNSName() ZonedDNSSetName {
	return ZonedDNSSetName{ZoneID: this.ZoneId(), DNSSetName: this.dnsSetName}
}

func (this *EntryVersion) Targets() Targets {
	return this.targets
}

func (this *EntryVersion) RoutingPolicy() *dns.RoutingPolicy {
	return this.routingPolicy
}

func (this *EntryVersion) Description() string {
	return this.object.Description()
}

func (this *EntryVersion) TTL() int64 {
	return utils.Int64Value(this.status.TTL, 0)
}

func (this *EntryVersion) Interval() int64 {
	return this.interval
}

func (this *EntryVersion) IsResponsible() bool {
	return this.responsible
}

func (this *EntryVersion) ProviderType() string {
	return utils.StringValue(this.status.ProviderType)
}

func (this *EntryVersion) ProviderName() resources.ObjectName {
	return this.providername
}

func (this *EntryVersion) OwnerId() string {
	if this.object.GetOwnerId() != nil {
		return *this.object.GetOwnerId()
	}
	return ""
}

func complete(logger logger.LogContext, state *state, entry *dnsutils.DNSEntryObject, prefix string) (*api.DNSEntrySpec, error) {
	if ref := entry.GetReference(); ref != nil && ref.Name != "" {
		newSpec := entry.Spec().DeepCopy()
		ns := ref.Namespace
		if ns == "" {
			ns = entry.GetNamespace()
		}
		dnsref := resources.NewObjectName(ns, ref.Name)
		logger.Infof("completeing spec by reference: %s%s", prefix, dnsref)

		cur := entry.ClusterKey()
		key := resources.NewClusterKey(cur.Cluster(), cur.GroupKind(), dnsref.Namespace(), dnsref.Name())
		state.references.AddRef(cur, key)

		ref, err := entry.GetResource().GetCached(dnsref)
		if err != nil {
			if errors.IsNotFound(err) {
				err = fmt.Errorf("entry reference %s%q not found", prefix, dnsref)
			}
			logger.Warn(err)
			return nil, err
		}
		err = access.CheckAccessWithRealms(entry, "use", ref, state.realms)
		if err != nil {
			return nil, fmt.Errorf("%s%s", prefix, err)
		}
		rspec, err := complete(logger, state, dnsutils.DNSEntry(ref), fmt.Sprintf("%s%s->", prefix, dnsref))
		if err != nil {
			return nil, err
		}

		if entry.GetTargets() != nil {
			return nil, fmt.Errorf("%stargets specified together with entry reference", prefix)
		}
		if entry.GetText() != nil {
			err = fmt.Errorf("%stext specified together with entry reference", prefix)
			return nil, err
		}
		newSpec.Targets = rspec.Targets
		newSpec.Text = rspec.Text

		if entry.GetTTL() == nil {
			newSpec.TTL = rspec.TTL
		}
		if entry.GetCNameLookupInterval() == nil {
			newSpec.CNameLookupInterval = rspec.CNameLookupInterval
		}
		return newSpec, nil
	} else {
		state.references.DelRef(entry.ClusterKey())
	}
	return entry.Spec(), nil
}

func validate(logger logger.LogContext, state *state, entry *EntryVersion, p *EntryPremise) (effspec *api.DNSEntrySpec, targets Targets, warnings []string, err error) {
	targets = Targets{}
	warnings = []string{}

	if !state.config.DisableDNSNameValidation {
		name := entry.object.GetDNSName()
		if err = dns.ValidateDomainName(name); err != nil {
			return
		}
	}

	effspec, err = complete(logger, state, entry.object, "")
	if err != nil {
		return
	}

	if p.zonedomain == entry.dnsSetName.DNSName {
		for _, t := range []string{"azure-dns", "azure-private-dns"} {
			if p.provider != nil && p.provider.TypeCode() == t {
				err = fmt.Errorf("usage of dns name (%s) identical to domain of hosted zone (%s) is not supported. Please use apex prefix '@.'",
					p.zonedomain, p.zoneid)
				return
			}
		}
	}
	if len(effspec.Targets) > 0 && len(effspec.Text) > 0 {
		err = fmt.Errorf("only Text or Targets possible")
		return
	}
	if ttl := effspec.TTL; ttl != nil && (*ttl == 0 || *ttl < 0) {
		err = fmt.Errorf("TTL must be greater than zero")
		return
	}

	for i, t := range effspec.Targets {
		if strings.TrimSpace(t) == "" {
			err = fmt.Errorf("target %d must not be empty", i+1)
			return
		}
		var new Target
		new, err = NewHostTargetFromEntryVersion(t, entry)
		if err != nil {
			return
		}
		if targets.Has(new) {
			warnings = append(warnings, fmt.Sprintf("dns entry %q has duplicate target %q", entry.ObjectName(), new))
		} else {
			targets = append(targets, new)
		}
	}
	tcnt := 0
	for _, t := range effspec.Text {
		if t == "" {
			warnings = append(warnings, fmt.Sprintf("dns entry %q has empty text", entry.ObjectName()))
			continue
		}
		new := dnsutils.NewText(t, entry.TTL())
		if targets.Has(new) {
			warnings = append(warnings, fmt.Sprintf("dns entry %q has duplicate text %q", entry.ObjectName(), new))
		} else {
			targets = append(targets, new)
			tcnt++
		}
	}
	if len(effspec.Text) > 0 && tcnt == 0 {
		err = fmt.Errorf("dns entry has only empty text")
		return
	}

	if len(targets) == 0 {
		err = fmt.Errorf("no target or text specified")
	}
	return
}

func (this *EntryVersion) Setup(logger logger.LogContext, state *state, p *EntryPremise, op string, err error, config Config) reconcile.Status {
	hello := dnsutils.NewLogMessage("%s ENTRY: %s, zoneid: %s, handler: %s, provider: %s, ref %+v", op, this.Object().Status().State, p.zoneid, p.ptype, Provider(p.provider), this.Object().GetReference())

	this.valid = false
	this.responsible = false
	spec := this.object.Spec()

	///////////// handle type responsibility

	if !utils.IsEmptyString(this.object.Status().ProviderType) && p.ptype == "" {
		// other controller claimed responsibility?
		this.status.ProviderType = this.object.Status().ProviderType
	}

	if utils.IsEmptyString(this.status.ProviderType) || (p.zoneid != "" && *this.status.ProviderType != p.ptype) {
		if p.zoneid == "" {
			// mark unassigned foreign entries as erroneous
			if this.object.GetCreationTimestamp().Add(config.RescheduleDelay).After(time.Now()) {
				if err := state.RemoveFinalizer(this.object); err != nil {
					return reconcile.Failed(logger, err)
				}
				return reconcile.Succeeded(logger).RescheduleAfter(config.RescheduleDelay)
			}
			hello.Infof(logger)
			if err != nil {
				logger.Infof("probably no responsible controller found (%s) -> mark as error", err)
			} else {
				logger.Info("probably no responsible controller found -> mark as error")
			}
			this.status.Provider = nil
			this.status.ProviderType = nil
			this.status.Zone = nil
			msg := "No responsible provider found"
			if err != nil {
				msg = fmt.Sprintf("%s: %s", msg, err)
			}
			err := this.updateStatus(logger, api.STATE_ERROR, "%s", msg)
			if err != nil {
				return reconcile.Delay(logger, err)
			}
		} else {
			// assign entry to actual type
			hello.Infof(logger)
			logger.Infof("assigning to provider type %q responsible for zone %s", p.ptype, p.zoneid)
			this.status.State = api.STATE_PENDING
			this.status.Message = StatusMessage("waiting for dns reconciliation")
		}
	}

	if p.zoneid == "" && !utils.IsEmptyString(this.status.ProviderType) && config.EnabledTypes.Contains(*this.status.ProviderType) {
		// revoke assignment to actual type
		oldType := utils.StringValue(this.status.ProviderType)
		hello.Infof(logger, "revoke assignment to %s", oldType)
		this.status.Provider = nil
		this.status.ProviderType = nil
		this.status.Zone = nil
		err := this.updateStatus(logger, "", "not valid for known provider anymore -> releasing provider type %s", oldType)
		if err != nil {
			return reconcile.Delay(logger, err)
		}
	}

	if p.zoneid == "" || p.ptype == "" {
		return reconcile.RepeatOnError(logger, state.RemoveFinalizer(this.object))
	}

	provider := ""
	this.status.Zone = &p.zoneid
	this.status.ProviderType = &p.ptype
	this.responsible = true
	if p.provider != nil {
		this.providername = p.provider.ObjectName()
		provider = p.provider.ObjectName().String()
		this.status.Provider = &provider
		defaultTTL := p.provider.DefaultTTL()
		this.status.TTL = &defaultTTL
		if spec.TTL != nil {
			this.status.TTL = spec.TTL
		}
	} else {
		this.providername = nil
		this.status.Provider = nil
		this.status.TTL = nil
	}

	///////////// validate

	spec, targets, warnings, verr := validate(logger, state, this, p)
	if verr != nil {
		hello.Infof(logger, "validation failed: %s", verr)

		_, _ = this.UpdateStatus(logger, api.STATE_INVALID, verr.Error())
		return reconcile.Failed(logger, verr)
	}

	///////////// handle

	hello.Infof(logger, "validation ok")

	if p.provider != nil && spec.TTL != nil {
		this.status.TTL = spec.TTL
	}

	if this.IsDeleting() {
		logger.Infof("update state to %s", api.STATE_DELETING)
		this.status.State = api.STATE_DELETING
		this.status.Message = StatusMessage("entry is scheduled to be deleted")
		this.valid = true
		state.DeleteLookupJob(this.object.ObjectName())
	} else {
		this.warnings = warnings
		targets, lookupResults, multiCName := normalizeTargets(logger, this.object, targets...)
		if multiCName {
			this.interval = int64(600)
			if iv := spec.CNameLookupInterval; iv != nil && *iv > 0 {
				this.interval = *iv
				if this.interval < 30 {
					this.interval = 30
				}
				if len(targets) > 0 && this.interval < targets[0].GetTTL()/3 {
					this.interval = targets[0].GetTTL() / 3
				}
			}
			if lookupResults != nil {
				state.UpsertLookupJob(this.object.ObjectName(), *lookupResults, time.Duration(this.interval)*time.Second)
			} else {
				state.DeleteLookupJob(this.object.ObjectName())
			}
			if len(targets) == 0 {
				msg := "targets cannot be resolved to any valid IPv4 address"
				if lookupResults == nil {
					msg = "too many targets"
					this.interval = int64(84600)
				}

				verr := fmt.Errorf("%s", msg)
				hello.Infof(logger, msg)

				state := api.STATE_INVALID
				// if DNS lookup fails temporarily, go to state STALE
				if this.status.State == api.STATE_READY || this.status.State == api.STATE_STALE {
					state = api.STATE_STALE
				}
				if _, err := this.UpdateStatus(logger, state, verr.Error()); err != nil {
					return reconcile.Failed(logger, err)
				}
				return reconcile.Recheck(logger, verr, time.Duration(this.interval)*time.Second)
			}
		} else {
			state.DeleteLookupJob(this.object.ObjectName())
			this.interval = 0
		}

		this.targets = targets
		this.routingPolicy = dnsutils.ToDNSRoutingPolicy(spec.RoutingPolicy)
		if err != nil {
			if this.status.State != api.STATE_STALE {
				if this.status.State == api.STATE_READY && (p.provider != nil && !p.provider.IsValid()) {
					this.status.State = api.STATE_STALE
				} else {
					this.status.State = api.STATE_ERROR
				}
				this.status.Message = StatusMessage(err.Error())
			} else {
				if strings.HasPrefix(*this.status.Message, MSG_PRESERVED) {
					this.status.Message = StatusMessage(MSG_PRESERVED + ": " + err.Error())
				} else {
					this.status.Message = StatusMessage(err.Error())
				}
			}
		} else {
			if p.zoneid == "" {
				this.status.State = api.STATE_ERROR
				this.status.Provider = nil
				this.status.Message = StatusMessagef("no provider found for %q", this.dnsSetName)
			} else {
				if p.provider.IsValid() {
					this.valid = true
				} else {
					this.status.State = api.STATE_STALE
					this.status.Message = StatusMessagef("provider %q not valid", p.provider.ObjectName())
				}
			}
		}

		if this.status.State == api.STATE_READY && this.object.Status() != nil && this.object.GetGeneration() != this.object.Status().ObservedGeneration {
			this.status.State = api.STATE_PENDING
		}
	}

	logger.Infof("%s: valid: %t, message: %s%s", this.status.State, this.valid, utils.StringValue(this.status.Message), errorValue(", err: %s", err))
	logmsg := dnsutils.NewLogMessage("update entry status")
	f := func(data resources.ObjectData) (bool, error) {
		obj, err := this.object.GetResource().Wrap(data)
		if err != nil {
			return false, err
		}
		status := dnsutils.DNSEntry(obj).Status()
		mod := &utils.ModificationState{}
		if p.zoneid != "" {
			mod.AssureStringPtrValue(&status.ProviderType, p.ptype)
		}
		mod.AssureStringValue(&status.State, this.status.State).
			AssureStringPtrPtr(&status.Message, this.status.Message).
			AssureStringPtrPtr(&status.Zone, this.status.Zone).
			AssureStringPtrPtr(&status.Provider, this.status.Provider)
		if mod.IsModified() {
			dnsutils.SetLastUpdateTime(&status.LastUpdateTime)
			logmsg.Infof(logger)
		}
		mod.Modify(dnsutils.DNSEntry(obj).AcknowledgeCNAMELookupInterval(this.interval))
		return mod.IsModified(), nil
	}
	_, err = this.object.ModifyStatus(f)

	return reconcile.DelayOnError(logger, err)
}

// NotRateLimited checks for annotation dns.gardener.cloud/not-rate-limited
func (this *EntryVersion) NotRateLimited() bool {
	value, ok := resources.GetAnnotation(this.object.Data(), dns.NOT_RATE_LIMITED_ANNOTATION)
	if ok {
		ok, _ = strconv.ParseBool(value)
	}
	return ok
}

func (this *EntryVersion) updateStatus(logger logger.LogContext, state, msg string, args ...interface{}) error {
	logmsg := dnsutils.NewLogMessage(msg, args...)
	f := func(data resources.ObjectData) (bool, error) {
		tmp, err := this.object.GetResource().Wrap(data)
		if err != nil {
			return false, err
		}
		o := dnsutils.DNSEntry(tmp)
		status := o.Status()
		mod := (&utils.ModificationState{}).
			AssureStringPtrPtr(&status.ProviderType, this.status.ProviderType).
			AssureStringValue(&status.State, state).
			AssureStringPtrValue(&status.Message, logmsg.Get()).
			AssureStringPtrPtr(&status.Zone, this.status.Zone).
			AssureStringPtrPtr(&status.Provider, this.status.Provider).
			AssureInt64PtrPtr(&status.TTL, this.status.TTL)
		if state != "" && status.ObservedGeneration < this.object.GetGeneration() {
			mod.AssureInt64Value(&status.ObservedGeneration, this.object.GetGeneration())
		}
		if utils.StringValue(this.status.Provider) == "" {
			mod.Modify(o.AcknowledgeTargets(nil))
			mod.Modify(o.AcknowledgeRoutingPolicy(nil))
		}
		if mod.IsModified() {
			logmsg.Infof(logger)
		}
		return mod.IsModified(), nil
	}
	_, err := this.object.ModifyStatus(f)
	this.object.Event(corev1.EventTypeNormal, "reconcile", logmsg.Get())
	return err
}

func (this *EntryVersion) UpdateStatus(logger logger.LogContext, state string, msg string) (bool, error) {
	f := func(data resources.ObjectData) (bool, error) {
		obj, err := this.object.GetResource().Wrap(data)
		if err != nil {
			return false, err
		}
		o := dnsutils.DNSEntry(obj)
		b := o.Status()
		if state == api.STATE_PENDING && b.State != "" {
			return false, nil
		}
		mod := &utils.ModificationState{}

		if state == api.STATE_READY {
			mod.AssureInt64PtrPtr(&b.TTL, this.status.TTL)
			targets := targetList(this.targets)
			if o.AcknowledgeTargets(targets) {
				logger.Infof("update effective targets: [%s]", strings.Join(targets, ", "))
				mod.Modify(true)
			}
			if o.AcknowledgeRoutingPolicy(this.routingPolicy) {
				mod.Modify(true)
			}
			if this.status.Provider != nil {
				mod.AssureStringPtrPtr(&b.Provider, this.status.Provider)
			}
		} else if state != api.STATE_STALE {
			mod.Modify(o.AcknowledgeTargets(nil))
			mod.Modify(o.AcknowledgeRoutingPolicy(nil))
		}
		mod.AssureInt64Value(&b.ObservedGeneration, o.GetGeneration())
		if !(this.status.State == api.STATE_STALE && this.status.State == state) {
			mod.AssureStringPtrValue(&b.Message, msg)
			this.status.Message = &msg
		}
		mod.AssureStringValue(&b.State, state)
		this.status.State = state
		if mod.IsModified() {
			dnsutils.SetLastUpdateTime(&b.LastUpdateTime)
			logger.Infof("update state of '%s/%s' to %s (%s)", o.GetNamespace(), o.GetName(), state, msg)
		}
		return mod.IsModified(), nil
	}
	return this.object.ModifyStatus(f)
}

func (this *EntryVersion) UpdateState(logger logger.LogContext, state, msg string) (bool, error) {
	f := func(data resources.ObjectData) (bool, error) {
		obj, err := this.object.GetResource().Wrap(data)
		if err != nil {
			return false, err
		}
		o := dnsutils.DNSEntry(obj)
		b := o.Status()
		mod := &utils.ModificationState{}

		mod.AssureStringPtrValue(&b.Message, msg)
		this.status.Message = &msg
		mod.AssureStringValue(&b.State, state)
		this.status.State = state
		if mod.IsModified() {
			dnsutils.SetLastUpdateTime(&b.LastUpdateTime)
			logger.Infof("update state of '%s/%s' to %s (%s)", o.GetNamespace(), o.GetName(), state, msg)
		}
		return mod.IsModified(), nil
	}
	return this.object.ModifyStatus(f)
}

func targetList(targets Targets) []string {
	targetList := make([]string, 0, len(targets))
	for _, t := range targets {
		targetList = append(targetList, t.GetHostName())
	}
	return targetList
}

func normalizeTargets(logger logger.LogContext, object *dnsutils.DNSEntryObject, targets ...Target) (Targets, *lookupAllResults, bool) {
	multiCNAME := len(targets) > 0 && targets[0].GetRecordType() == dns.RS_CNAME && (len(targets) > 1 || ptr.Deref(object.ResolveTargetsToAddresses(), false))
	if !multiCNAME {
		return targets, nil, false
	}

	if len(targets) > maxCNAMETargets {
		w := fmt.Sprintf("too many CNAME targets: %d (maximum allowed: %d)", len(targets), maxCNAMETargets)
		logger.Warn(w)
		object.Event(corev1.EventTypeWarning, "dnslookup restriction", w)
		return nil, nil, true
	}
	result := make(Targets, 0, len(targets))
	hostnames := make([]string, len(targets))
	for i, t := range targets {
		hostnames[i] = t.GetHostName()
	}
	ctx := context.Background()
	results := lookupAllHostnamesIPs(ctx, hostnames...)
	ttl := targets[0].GetTTL()
	for _, addr := range results.ipv4Addrs {
		result = append(result, dnsutils.NewTarget(dns.RS_A, addr, ttl))
	}
	for _, addr := range results.ipv6Addrs {
		result = append(result, dnsutils.NewTarget(dns.RS_AAAA, addr, ttl))
	}
	for _, err := range results.errs {
		logger.Warn(err.Error())
		object.Event(corev1.EventTypeNormal, "dnslookup", err.Error())
	}
	return result, &results, true
}

///////////////////////////////////////////////////////////////////////////////

type Entry struct {
	lock       *dnsutils.TryLock
	key        string
	createdAt  time.Time
	modified   bool
	activezone dns.ZoneID
	state      *state

	*EntryVersion
}

func NewEntry(v *EntryVersion, state *state) *Entry {
	e := &Entry{
		lock:         dnsutils.NewTryLock(state.GetContext().GetContext()),
		key:          v.ObjectName().String(),
		EntryVersion: v,
		state:        state,
		modified:     true,
		createdAt:    time.Now(),
	}
	if v.status.ProviderType != nil && v.status.Zone != nil {
		e.activezone = dns.NewZoneID(*v.status.ProviderType, *v.status.Zone)
	}
	return e
}

func (this *Entry) RemoveFinalizer() error {
	return this.state.RemoveFinalizer(this.object.DeepCopy())
}

func (this *Entry) Trigger(logger logger.LogContext) {
	this.state.TriggerEntry(logger, this)
}

func (this *Entry) IsModified() bool {
	return this.modified
}

func (this *Entry) CreatedAt() time.Time {
	return this.createdAt
}

func (this *Entry) Update(logger logger.LogContext, new *EntryVersion) *Entry {
	if this.ZonedDNSName() != new.ZonedDNSName() {
		return NewEntry(new, this.state)
	}

	reasons := this.RequiresUpdateFor(new)
	if len(reasons) != 0 {
		logger.Infof("update actual entry: valid: %t  %v", new.IsValid(), reasons)
		if this.targets.DifferFrom(new.targets) && !new.IsDeleting() {
			logger.Infof("targets differ from internal state")
			for _, w := range new.warnings {
				logger.Warn(w)
				this.object.Event(corev1.EventTypeNormal, "reconcile", w)
			}
			for dns, m := range new.mappings {
				msg := fmt.Sprintf("mapping cname %q to %v", dns, m)
				logger.Info(msg)
				this.object.Event(corev1.EventTypeNormal, "dnslookup", msg)
			}
			logger.Infof("update effective targets: [%s]", strings.Join(targetList(new.targets), ", "))
		}
		this.modified = true
	}
	this.EntryVersion = new

	if new.valid && this.status.State == api.STATE_STALE {
		this.modified = true
	}

	return this
}

func (this *Entry) Before(e *Entry) bool {
	if e == nil {
		return true
	}
	if this.Object().GetCreationTimestamp().Time.Equal(e.Object().GetCreationTimestamp().Time) {
		// for entries created at same time compare objectname to define strict order
		return strings.Compare(this.ObjectName().String(), e.ObjectName().String()) < 0
	}
	return this.Object().GetCreationTimestamp().Time.Before(e.Object().GetCreationTimestamp().Time)
}

func (this *Entry) updateStatistic(statistic *statistic.EntryStatistic) {
	if err := this.lock.Lock(); err != nil {
		return
	}
	defer this.lock.Unlock()
	statistic.Providers.Inc(this.ProviderType(), this.ProviderName())
}

////////////////////////////////////////////////////////////////////////////////
// Entries
////////////////////////////////////////////////////////////////////////////////

type Entries map[resources.ObjectName]*Entry

func (this Entries) AddResponsibleTo(list *EntryList) {
	for _, e := range this {
		if e.IsResponsible() {
			*list = append(*list, e)
		}
	}
}

func (this Entries) AddActiveZoneTo(zoneid dns.ZoneID, list *EntryList) {
	for _, e := range this {
		if e.activezone == zoneid {
			*list = append(*list, e)
		}
	}
}

func (this Entries) AddEntry(entry *Entry) *Entry {
	old := this[entry.ObjectName()]
	this[entry.ObjectName()] = entry
	if old != nil && old != entry {
		return old
	}
	return nil
}

func (this Entries) Delete(e *Entry) {
	if this[e.ObjectName()] == e {
		delete(this, e.ObjectName())
	}
}

////////////////////////////////////////////////////////////////////////////////
// synchronizedEntries
////////////////////////////////////////////////////////////////////////////////

type synchronizedEntries struct {
	lock    sync.Mutex
	entries Entries
}

func newSynchronizedEntries() *synchronizedEntries {
	return &synchronizedEntries{entries: Entries{}}
}

func (this *synchronizedEntries) AddResponsibleTo(list *EntryList) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.entries.AddResponsibleTo(list)
}

func (this *synchronizedEntries) AddActiveZoneTo(zoneid dns.ZoneID, list *EntryList) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.entries.AddActiveZoneTo(zoneid, list)
}

func (this *synchronizedEntries) AddEntry(entry *Entry) *Entry {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.entries.AddEntry(entry)
}

func (this *synchronizedEntries) Delete(e *Entry) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.entries.Delete(e)
}

////////////////////////////////////////////////////////////////////////////////
// EntryList
////////////////////////////////////////////////////////////////////////////////

type EntryList []*Entry

func (this EntryList) Len() int {
	return len(this)
}

func (this EntryList) Less(i, j int) bool {
	return strings.Compare(this[i].key, this[j].key) < 0
}

func (this EntryList) Swap(i, j int) {
	this[i], this[j] = this[j], this[i]
}

func (this EntryList) Sort() {
	sort.Sort(this)
}

func (this EntryList) Lock() error {
	this.Sort()
	for i := 0; i < len(this); i++ {
		err := this[i].lock.Lock()
		if err != nil {
			for j := i - 1; j >= 0; j-- {
				this[j].lock.Unlock()
			}
			return err
		}
	}
	return nil
}

func (this EntryList) Unlock() {
	for _, e := range this {
		e.lock.Unlock()
	}
}

func (this EntryList) UpdateStatistic(statistic *statistic.EntryStatistic) {
	for _, e := range this {
		e.updateStatistic(statistic)
	}
}

func StatusMessage(s string) *string {
	return &s
}

func StatusMessagef(msgfmt string, args ...interface{}) *string {
	return StatusMessage(fmt.Sprintf(msgfmt, args...))
}

func Provider(p DNSProvider) string {
	if p == nil {
		return "<none>"
	}
	return p.ObjectName().String()
}

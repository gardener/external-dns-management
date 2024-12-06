// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/dns"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var DNSEntryType = (*api.DNSEntry)(nil)

type DNSEntryObject struct {
	resources.Object
}

func (this *DNSEntryObject) DNSEntry() *api.DNSEntry {
	return this.Data().(*api.DNSEntry)
}

func DNSEntry(o resources.Object) *DNSEntryObject {
	if o.IsA(DNSEntryType) {
		return &DNSEntryObject{o}
	}
	return nil
}

func (this *DNSEntryObject) Spec() *api.DNSEntrySpec {
	return &this.DNSEntry().Spec
}

func (this *DNSEntryObject) StatusField() interface{} {
	return this.Status()
}

func (this *DNSEntryObject) Status() *api.DNSEntryStatus {
	return &this.DNSEntry().Status
}

func GetDNSName(entry *api.DNSEntry) string {
	return dns.NormalizeHostname(entry.Spec.DNSName)
}

func (this *DNSEntryObject) GetDNSName() string {
	return GetDNSName(this.DNSEntry())
}

func (this *DNSEntryObject) GetSetIdentifier() string {
	if policy := this.DNSEntry().Spec.RoutingPolicy; policy != nil {
		return policy.SetIdentifier
	}
	return ""
}

func (this *DNSEntryObject) GetTargets() []string {
	return this.DNSEntry().Spec.Targets
}

func (this *DNSEntryObject) GetText() []string {
	return this.DNSEntry().Spec.Text
}

func (this *DNSEntryObject) GetOwnerId() *string {
	return this.DNSEntry().Spec.OwnerId
}

func (this *DNSEntryObject) GetTTL() *int64 {
	return this.DNSEntry().Spec.TTL
}

func (this *DNSEntryObject) GetCNameLookupInterval() *int64 {
	return this.DNSEntry().Spec.CNameLookupInterval
}

func (this *DNSEntryObject) ResolveTargetsToAddresses() *bool {
	return this.DNSEntry().Spec.ResolveTargetsToAddresses
}

func (this *DNSEntryObject) GetReference() *api.EntryReference {
	return this.DNSEntry().Spec.Reference
}

func (this *DNSEntryObject) GetRoutingPolicy() *dns.RoutingPolicy {
	return ToDNSRoutingPolicy(this.DNSEntry().Spec.RoutingPolicy)
}

func (this *DNSEntryObject) AcknowledgeTargets(targets []string) bool {
	s := this.Status()
	if !reflect.DeepEqual(s.Targets, targets) {
		s.Targets = targets
		return true
	}
	return false
}

func (this *DNSEntryObject) AcknowledgeRoutingPolicy(policy *dns.RoutingPolicy) bool {
	s := this.Status()
	if s.RoutingPolicy == nil && policy == nil {
		return false
	}
	if policy == nil {
		s.RoutingPolicy = nil
		return true
	}
	statusPolicy := &api.RoutingPolicy{
		Type:          policy.Type,
		SetIdentifier: this.GetSetIdentifier(),
		Parameters:    policy.Parameters,
	}
	if !reflect.DeepEqual(s.RoutingPolicy, statusPolicy) {
		s.RoutingPolicy = statusPolicy
		return true
	}
	return false
}

func (this *DNSEntryObject) AcknowledgeCNAMELookupInterval(interval int64) bool {
	s := this.Status()
	if interval == 0 {
		mod := s.CNameLookupInterval != nil
		s.CNameLookupInterval = nil
		return mod
	}
	var mod bool
	s.CNameLookupInterval, mod = utils.AssureInt64PtrValue(false, s.CNameLookupInterval, interval)
	return mod
}

func (this *DNSEntryObject) GetTargetSpec(p TargetProvider) TargetSpec {
	return BaseTargetSpec(this, p)
}

func DNSSetName(entry *api.DNSEntry) dns.DNSSetName {
	setIdentifier := ""
	if policy := entry.Spec.RoutingPolicy; policy != nil {
		setIdentifier = policy.SetIdentifier
	}
	return dns.DNSSetName{
		DNSName:       GetDNSName(entry),
		SetIdentifier: setIdentifier,
	}
}

func (this *DNSEntryObject) DNSSetName() dns.DNSSetName {
	return DNSSetName(this.DNSEntry())
}

func DNSSetNameMatcher(name dns.DNSSetName) resources.ObjectMatcher {
	return func(o resources.Object) bool {
		return DNSEntry(o).DNSSetName() == name
	}
}

func ToDNSRoutingPolicy(policy *api.RoutingPolicy) *dns.RoutingPolicy {
	if policy != nil {
		return &dns.RoutingPolicy{
			Type:       policy.Type,
			Parameters: policy.Parameters,
		}
	}
	return nil
}

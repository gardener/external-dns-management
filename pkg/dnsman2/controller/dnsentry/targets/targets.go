// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package targets

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"k8s.io/utils/ptr"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/lookup"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// maxCNAMETargets is the maximum number of CNAME targets. It is restricted, as it needs regular DNS lookups.
const maxCNAMETargets = 25

// TargetsResult holds the result of extracting targets from a DNSEntrySpec.
type TargetsResult struct {
	EntryKey client.ObjectKey
	Targets  dns.Targets
	Warnings []string
}

// AddTarget adds a target to the TargetsResult, avoiding duplicates and adding a warning if duplicate.
func (r *TargetsResult) AddTarget(target dns.Target) {
	if r.Targets.Has(target) {
		field := "target"
		if target.GetRecordType() == dns.TypeTXT {
			field = "text"
		}
		r.AddWarning(fmt.Sprintf("dns Entry %q has duplicate %s %q", r.EntryKey, field, target))
	} else {
		r.Targets = append(r.Targets, target)
	}
}

// HasTargets returns true if there are any targets in the result.
func (r *TargetsResult) HasTargets() bool {
	return len(r.Targets) > 0
}

// HasWarnings returns true if there are any warnings in the result.
func (r *TargetsResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// AddWarning adds a warning message to the result.
func (r *TargetsResult) AddWarning(warning string) {
	r.Warnings = append(r.Warnings, warning)
}

// TargetsProducer is responsible for producing dns.Targets from a DNSEntrySpec.
type TargetsProducer struct {
	ctx                        context.Context
	defaultTTL                 int64
	defaultCNAMELookupInterval int64
	processor                  lookup.LookupProcessor
}

// NewTargetsProducer creates a new TargetsProducer.
func NewTargetsProducer(ctx context.Context, defaultTTL, defaultCNAMELookupInterval int64, processor lookup.LookupProcessor) *TargetsProducer {
	return &TargetsProducer{
		ctx:                        ctx,
		defaultTTL:                 defaultTTL,
		defaultCNAMELookupInterval: defaultCNAMELookupInterval,
		processor:                  processor,
	}
}

// FromSpec extracts dns.Targets from a DNSEntrySpec.
// It validates the spec and returns warnings for duplicate targets or empty text.
func (p *TargetsProducer) FromSpec(key client.ObjectKey, spec *v1alpha1.DNSEntrySpec, ipstack string) (result *TargetsResult, err error) {
	if err = dns.ValidateDomainName(spec.DNSName); err != nil {
		return
	}

	if spec.Reference != nil { //nolint:staticcheck // will be removed in a future release
		// TODO(MartinWeindel) document that reference field is not supported anymore
		err = fmt.Errorf("`reference` field is not supported anymore")
		return
	}

	if len(spec.Targets) > 0 && len(spec.Text) > 0 {
		err = fmt.Errorf("only text or targets possible")
		return
	}
	if ttl := spec.TTL; ttl != nil && (*ttl == 0 || *ttl < 0) {
		err = fmt.Errorf("TTL must be greater than zero")
		return
	}

	result = &TargetsResult{EntryKey: key}
	for i, t := range spec.Targets {
		if strings.TrimSpace(t) == "" {
			err = fmt.Errorf("target %d must not be empty", i+1)
			return
		}
		var newTarget dns.Target
		newTarget, err = newAddressTarget(t, ptr.Deref(spec.TTL, p.defaultTTL), ipstack)
		if err != nil {
			return
		}
		result.AddTarget(newTarget)
	}
	emptyCount := 0
	for _, t := range spec.Text {
		if t == "" {
			result.AddWarning(fmt.Sprintf("dns Entry %q has empty text", key))
			emptyCount++
			continue
		}
		newTarget := dns.NewText(t, ptr.Deref(spec.TTL, p.defaultTTL))
		result.AddTarget(newTarget)
	}
	if emptyCount > 0 && len(spec.Text) == emptyCount {
		err = fmt.Errorf("dns Entry has only empty text")
		return
	}

	if !result.HasTargets() {
		err = fmt.Errorf("no target or text specified")
		return
	}

	resolveTargetsToAddresses, err := checkCNAMETargets(spec, result.Targets)
	if err != nil {
		return
	}

	if resolveTargetsToAddresses {
		err = p.upsertLookupJob(p.ctx, key, spec, result)
		if err != nil {
			return
		}
		// TODO(MartinWeindel) update lookup processor, retrieve addresses for CNAME targets
	} else {
		err = p.deleteLookupJob(key)
		if err != nil {
			return
		}
	}

	return
}

func (p *TargetsProducer) upsertLookupJob(ctx context.Context, key client.ObjectKey, spec *v1alpha1.DNSEntrySpec, result *TargetsResult) error {
	if p.processor == nil {
		return fmt.Errorf("lookup processor is not initialized")
	}

	var ttl int64
	if len(result.Targets) > 0 {
		ttl = result.Targets[0].GetTTL()
	}
	lookupInterval := p.defaultCNAMELookupInterval
	if iv := spec.CNameLookupInterval; iv != nil && *iv > 0 {
		lookupInterval = *iv
		if lookupInterval < 30 {
			lookupInterval = 30
		}
		if len(result.Targets) > 0 {
			if ttl > 0 && lookupInterval < ttl/3 {
				lookupInterval = ttl / 3
			}
		}
	}

	hostnames := make([]string, 0, len(result.Targets))
	for _, target := range result.Targets {
		if target.GetRecordType() != dns.TypeCNAME {
			return fmt.Errorf("unexpected target type %s for CNAME lookup", target.GetRecordType())
		}
		hostnames = append(hostnames, target.GetRecordValue())
	}

	lookupAllResults := lookup.LookupAllHostnamesIPs(ctx, hostnames...)
	p.processor.Upsert(ctx, key, lookupAllResults, time.Duration(lookupInterval)*time.Second)
	if lookupAllResults.HasErrors() && !lookupAllResults.HasOnlyNotFoundError() {
		return fmt.Errorf("lookup failed for some targets: %s", errors.Join(lookupAllResults.Errs...))
	}

	var resolvedTargets dns.Targets
	for _, ip := range lookupAllResults.IPv4Addrs {
		resolvedTargets = append(resolvedTargets, dns.NewTarget(dns.TypeA, ip, ttl))
	}
	for _, ip := range lookupAllResults.IPv6Addrs {
		resolvedTargets = append(resolvedTargets, dns.NewTarget(dns.TypeAAAA, ip, ttl))
	}
	result.Targets = resolvedTargets
	return nil
}

func (p *TargetsProducer) deleteLookupJob(key client.ObjectKey) error {
	if p.processor == nil {
		return nil
	}
	p.processor.Delete(key)
	return nil
}

// checkCNAMETargets checks if the targets contain CNAME records and returns true if CNAME should be resolved to addresses.
func checkCNAMETargets(spec *v1alpha1.DNSEntrySpec, targets dns.Targets) (bool, error) {
	cnameCount := 0
	otherCount := 0
	for _, t := range targets {
		if t.GetRecordType() == dns.TypeCNAME {
			cnameCount++
		} else {
			otherCount++
		}
	}
	if cnameCount > 0 && otherCount > 0 {
		return false, fmt.Errorf("cannot mix CNAME and other record types in targets")
	}
	if cnameCount > maxCNAMETargets {
		return false, fmt.Errorf("too many CNAME targets (%d), maximum is %d", cnameCount, maxCNAMETargets)
	}
	resolveTargetsToAddresses := cnameCount > 1 || (cnameCount == 1 && ptr.Deref(spec.ResolveTargetsToAddresses, false))
	return resolveTargetsToAddresses, nil
}

// StatusToTargets converts a DNSEntryStatus to dns.Targets.
func StatusToTargets(status *v1alpha1.DNSEntryStatus, ipstack string) (targets dns.Targets, err error) {
	if status.Zone == nil {
		// no zone set, so no DNS records applied yet
		return
	}

	ttl := ptr.Deref(status.TTL, 0)
	for _, t := range status.Targets {
		var newTarget dns.Target
		if strings.HasPrefix(t, `"`) {
			unquoted, err2 := strconv.Unquote(t)
			if err2 != nil {
				return nil, fmt.Errorf("failed to unquote TXT target %s: %w", t, err2)
			}
			newTarget = dns.NewTarget(dns.TypeTXT, unquoted, ttl)
		} else {
			newTarget, err = newAddressTarget(t, ttl, ipstack)
			if err != nil {
				return
			}
		}
		if !targets.Has(newTarget) {
			targets = append(targets, newTarget)
		}
	}
	return
}

// TargetsToStrings converts values of dns.Targets to a slice of strings.
func TargetsToStrings(targets dns.Targets) []string {
	strs := make([]string, len(targets))
	for i, t := range targets {
		value := t.GetRecordValue()
		if t.GetRecordType() == dns.TypeTXT {
			value = strconv.Quote(value)
		}
		strs[i] = value
	}
	if len(targets) > 1 && targets[0].GetRecordType() != dns.TypeTXT {
		strs = makeUniqueStrings(strs)
	}
	return strs
}

func newAddressTarget(name string, ttl int64, ipstack string) (dns.Target, error) {
	ip := net.ParseIP(name)
	if ip == nil {
		return dns.NewTargetWithIPStack(dns.TypeCNAME, name, ttl, ipstack), nil
	} else if ip.To4() != nil {
		return dns.NewTarget(dns.TypeA, name, ttl), nil
	} else if ip.To16() != nil {
		return dns.NewTarget(dns.TypeAAAA, name, ttl), nil
	} else {
		return nil, fmt.Errorf("unexpected IP address (neither IPv4 or IPv6): %s (%s)", ip.String(), name)
	}
}

// makeUniqueStrings removes duplicates from a slice of strings and returns a new slice.
func makeUniqueStrings(array []string) []string {
	uniqArray := make([]string, 0, len(array))
	for _, str := range array {
		if !slices.Contains(uniqArray, str) {
			uniqArray = append(uniqArray, str)
		}
	}
	return uniqArray
}

// TODO(MartinWeindel) move this check to the provider
//if p.zonedomain == Entry.dnsSetName.DNSName {
//	for _, t := range []string{"azure-dns", "azure-private-dns"} {
//		if p.provider != nil && p.provider.TypeCode() == t {
//			Err = fmt.Errorf("usage of dns name (%s) identical to domain of hosted zone (%s) is not supported. Please use apex prefix '@.'", p.zonedomain, p.zoneid)
//			return
//		}
//	}
//}

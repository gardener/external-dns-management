// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// SpecToTargets converts a DNSEntrySpec to dns.Targets.
// It validates the spec and returns warnings for duplicate targets or empty text.
func SpecToTargets(key client.ObjectKey, spec *v1alpha1.DNSEntrySpec, ipstack string, defaultTTL int64) (targets dns.Targets, warnings []string, err error) {
	if err = dns.ValidateDomainName(spec.DNSName); err != nil {
		return
	}

	if spec.Reference != nil {
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

	for i, t := range spec.Targets {
		if strings.TrimSpace(t) == "" {
			err = fmt.Errorf("target %d must not be empty", i+1)
			return
		}
		var newTarget dns.Target
		newTarget, err = newAddressTarget(t, ptr.Deref(spec.TTL, defaultTTL), ipstack)
		if err != nil {
			return
		}
		if targets.Has(newTarget) {
			warnings = append(warnings, fmt.Sprintf("dns entry %q has duplicate target %q", key, newTarget))
		} else {
			targets = append(targets, newTarget)
		}
	}
	count := 0
	for _, t := range spec.Text {
		if t == "" {
			warnings = append(warnings, fmt.Sprintf("dns entry %q has empty text", key))
			continue
		}
		newTarget := dns.NewText(t, ptr.Deref(spec.TTL, defaultTTL))
		if targets.Has(newTarget) {
			warnings = append(warnings, fmt.Sprintf("dns entry %q has duplicate text %q", key, newTarget))
		} else {
			targets = append(targets, newTarget)
			count++
		}
	}
	if len(spec.Text) > 0 && count == 0 {
		err = fmt.Errorf("dns entry has only empty text")
		return
	}

	if len(targets) == 0 {
		err = fmt.Errorf("no target or text specified")
		return
	}

	if err = checkCNAMETargets(targets); err != nil {
		return
	}

	return
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

func checkCNAMETargets(targets dns.Targets) error {
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
		return fmt.Errorf("cannot mix CNAME and other record types in targets")
	}
	if cnameCount > 1 {
		return fmt.Errorf("cannot have multiple CNAME targets")
	}
	return nil
}

// TODO(MartinWeindel) move this check to the provider
//if p.zonedomain == entry.dnsSetName.DNSName {
//	for _, t := range []string{"azure-dns", "azure-private-dns"} {
//		if p.provider != nil && p.provider.TypeCode() == t {
//			err = fmt.Errorf("usage of dns name (%s) identical to domain of hosted zone (%s) is not supported. Please use apex prefix '@.'", p.zonedomain, p.zoneid)
//			return
//		}
//	}
//}

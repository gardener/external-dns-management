// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"fmt"
	"strings"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/selection"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

type providerSelector struct {
	entryContext
	namespace string
	class     string
	state     *state.State
}

type newProviderData struct {
	provider      *v1alpha1.DNSProvider
	providerKey   client.ObjectKey
	zoneID        dns.ZoneID
	defaultTTL    int64
	providerState *state.ProviderState
}

func (s *providerSelector) calcNewProvider() (*newProviderData, *reconcileResult) {
	newProvider, err := s.findBestMatchingProvider(s.entry.Spec.DNSName, s.entry.Status.Provider)
	if err != nil {
		s.log.Error(err, "failed to find a matching DNS provider for the entry")
		return nil, &reconcileResult{err: err}
	}
	if newProvider != nil {
		providerKey := client.ObjectKeyFromObject(newProvider)
		newZoneID, res := s.getZoneForProvider(newProvider, s.entry.Spec.DNSName)
		if res != nil {
			s.log.Error(err, "failed to get zone for provider", "provider", providerKey)
			return nil, res
		}
		if res := s.statusUpdater().addFinalizer(); res != nil {
			return nil, res
		}
		providerState := s.state.GetProviderState(providerKey)
		if providerState == nil {
			s.log.Error(err, "failed to get provider state", "provider", providerKey)
			return nil, &reconcileResult{err: err}
		}

		return &newProviderData{
			provider:      newProvider,
			providerKey:   providerKey,
			zoneID:        *newZoneID,
			providerState: providerState,
			defaultTTL:    providerState.GetDefaultTTL(),
		}, nil
	}
	return nil, nil
}

func (s *providerSelector) findBestMatchingProvider(dnsName string, currentProviderName *string) (*v1alpha1.DNSProvider, error) {
	providerList := &v1alpha1.DNSProviderList{}
	if err := s.client.List(s.ctx, providerList, client.InNamespace(s.namespace)); err != nil {
		return nil, err
	}
	return findBestMatchingProvider(dns.FilterProvidersByClass(providerList.Items, s.class), dnsName, currentProviderName)
}

func (s *providerSelector) getZoneForProvider(provider *v1alpha1.DNSProvider, dnsName string) (*dns.ZoneID, *reconcileResult) {
	pstate := s.state.GetProviderState(client.ObjectKeyFromObject(provider))
	if pstate == nil {
		return nil, &reconcileResult{result: reconcile.Result{Requeue: true}} // Provider state not yet available, requeue to wait for its reconciliation
	}
	var (
		bestZone  selection.LightDNSHostedZone
		bestMatch int
	)
	for _, zone := range pstate.GetSelection().Zones {
		if m := matchDomains(dnsName, []string{zone.Domain()}); m > bestMatch {
			bestMatch = m
			bestZone = zone
		}
	}
	if bestZone == nil {
		return nil, &reconcileResult{err: fmt.Errorf("no matching zone found for DNS name %q in provider %q", dnsName, provider.Name)}
	}
	return ptr.To(bestZone.ZoneID()), nil
}

type providerMatch struct {
	found *v1alpha1.DNSProvider
	match int
}

func findBestMatchingProvider(providers []v1alpha1.DNSProvider, dnsName string, currentProviderName *string) (*v1alpha1.DNSProvider, error) {
	handleMatch := func(match *providerMatch, p *v1alpha1.DNSProvider, n int, err error) error {
		if match.match > n {
			return err
		}
		if match.match < n || ptr.Deref(currentProviderName, "") == p.Name {
			match.found = p
			match.match = n
			return nil
		}
		return err
	}
	var err error
	validMatch := &providerMatch{}
	errorMatch := &providerMatch{}
	for _, p := range providers {
		n := matchSelection(dnsName, p.Status.Domains)
		if n > 0 {
			if p.Status.State == v1alpha1.StateReady {
				err = handleMatch(validMatch, &p, n, err)
			} else {
				err = handleMatch(errorMatch, &p, n, err)
			}
		}
	}
	if validMatch.found != nil {
		return validMatch.found, nil
	}
	if errorMatch.found != nil {
		return errorMatch.found, nil
	}
	return nil, err
}

func matchSelection(name string, selection v1alpha1.DNSSelectionStatus) int {
	ilen := matchDomains(name, selection.Included)
	elen := matchDomains(name, selection.Excluded)
	if ilen > elen {
		return ilen
	}
	return 0
}

func matchDomains(name string, domains []string) int {
	length := 0
	for _, d := range domains {
		if len(d) > length && matchesSuffix(name, d) {
			length = len(d)
		}
	}
	return length
}

func matchesSuffix(name, suffix string) bool {
	return name == suffix || strings.HasSuffix(name, "."+suffix)
}

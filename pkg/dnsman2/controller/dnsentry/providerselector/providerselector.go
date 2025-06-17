// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package providerselector

import (
	"fmt"
	"strings"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/selection"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// NewProviderData holds the data for a new DNS provider that is selected for a DNSEntry.
type NewProviderData struct {
	Provider      *v1alpha1.DNSProvider
	ProviderKey   client.ObjectKey
	ZoneID        dns.ZoneID
	DefaultTTL    int64
	ProviderState *state.ProviderState
}

// CalcNewProvider is a utility function to calculate a new DNS provider for the given EntryContext, namespace, and class.
func CalcNewProvider(ec common.EntryContext, namespace, class string, state *state.State) (*NewProviderData, *common.ReconcileResult) {
	selector := providerSelector{
		EntryContext: ec,
		namespace:    namespace,
		class:        class,
		state:        state,
	}
	return selector.calcNewProvider()
}

type providerSelector struct {
	common.EntryContext
	namespace string
	class     string
	state     *state.State
}

func (s *providerSelector) calcNewProvider() (*NewProviderData, *common.ReconcileResult) {
	newProvider, err := s.findBestMatchingProvider(s.Entry.Spec.DNSName, s.Entry.Status.Provider)
	if err != nil {
		s.Log.Error(err, "failed to find a matching DNS provider for the Entry")
		return nil, &common.ReconcileResult{Err: err}
	}
	if newProvider != nil {
		providerKey := client.ObjectKeyFromObject(newProvider)
		newZoneID, res := s.getZoneForProvider(newProvider, s.Entry.Spec.DNSName)
		if res != nil {
			s.Log.Error(err, "failed to get zone for provider", "provider", providerKey)
			return nil, res
		}
		if res := s.StatusUpdater().AddFinalizer(); res != nil {
			return nil, res
		}
		providerState := s.state.GetProviderState(providerKey)
		if providerState == nil {
			s.Log.Error(err, "failed to get provider state", "provider", providerKey)
			return nil, &common.ReconcileResult{Err: err}
		}

		return &NewProviderData{
			Provider:      newProvider,
			ProviderKey:   providerKey,
			ZoneID:        *newZoneID,
			ProviderState: providerState,
			DefaultTTL:    providerState.GetDefaultTTL(),
		}, nil
	}
	return nil, nil
}

func (s *providerSelector) findBestMatchingProvider(dnsName string, currentProviderName *string) (*v1alpha1.DNSProvider, error) {
	providerList := &v1alpha1.DNSProviderList{}
	if err := s.Client.List(s.Ctx, providerList, client.InNamespace(s.namespace)); err != nil {
		return nil, err
	}
	return findBestMatchingProvider(dns.FilterProvidersByClass(providerList.Items, s.class), dnsName, currentProviderName)
}

func (s *providerSelector) getZoneForProvider(provider *v1alpha1.DNSProvider, dnsName string) (*dns.ZoneID, *common.ReconcileResult) {
	pstate := s.state.GetProviderState(client.ObjectKeyFromObject(provider))
	if pstate == nil {
		return nil, &common.ReconcileResult{Result: reconcile.Result{Requeue: true}} // Provider state not yet available, requeue to wait for its reconciliation
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
		return nil, &common.ReconcileResult{Err: fmt.Errorf("no matching zone found for DNS name %q in provider %q", dnsName, provider.Name)}
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
		if len(d) > length && MatchesSuffix(name, d) {
			length = len(d)
		}
	}
	return length
}

// MatchesSuffix checks if the given name matches the suffix, either as an exact match or as a subdomain.
func MatchesSuffix(name, suffix string) bool {
	return name == suffix || strings.HasSuffix(name, "."+suffix)
}

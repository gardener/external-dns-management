// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/selection"
	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

func (r *Reconciler) reconcile(ctx context.Context, log logr.Logger, entry *v1alpha1.DNSEntry) (reconcile.Result, error) {
	provider, err := r.findBestMatchingProvider(ctx, entry.Spec.DNSName, entry.Status.Provider)
	if err != nil {
		log.Error(err, "failed to find a matching DNS provider for the entry", "entry", entry.Name)
		return reconcile.Result{}, err
	}
	if provider != nil {
		zone, result, err := r.getZoneForProvider(provider, entry.Spec.DNSName)
		if result != nil {
			log.Error(err, "failed to get zone for provider", "provider", provider.Name, "entry", entry.Name)
			return *result, err
		}
		if err != nil {
			log.Error(err, "failed to get zone for provider", "provider", provider.Name, "entry", entry.Name)
			return reconcile.Result{}, err
		}
		if err := addFinalizer(ctx, r.Client, entry); err != nil {
			log.Error(err, "failed to add finalizer to DNSEntry", "entry", entry.Name)
			return reconcile.Result{}, err
		}
		if err := r.updateStatus(ctx, entry, func(status *v1alpha1.DNSEntryStatus) error {
			status.Provider = &provider.Name
			status.ProviderType = ptr.To(provider.Spec.Type)
			status.Zone = zone
			status.Targets = []string{entry.Spec.DNSName} // TODO(MartinWeindel) Placeholder for actual targets, should be replaced with real logic to fetch targets
			status.ObservedGeneration = entry.Generation
			return nil
		}); err != nil {
			log.Error(err, "failed to update DNSEntry status", "entry", entry.Name)
			return reconcile.Result{}, err
		}
	} else {
		if err := r.updateStatus(ctx, entry, func(status *v1alpha1.DNSEntryStatus) error {
			status.Provider = nil
			status.ProviderType = nil
			status.ObservedGeneration = entry.Generation
			if !reflect.DeepEqual(status.Targets, []string{entry.Spec.DNSName}) {
				// TODO(MartinWeindel): Placeholder logic, must delete old DNS records if targets change
				status.Targets = nil // Clear targets if no provider is found
				status.Zone = nil    // Clear zone if no provider is found
			}
			return nil
		}); err != nil {
			log.Error(err, "failed to update DNSEntry status", "entry", entry.Name)
			return reconcile.Result{}, err
		}
		if len(entry.Status.Targets) == 0 {
			if err := removeFinalizer(ctx, r.Client, entry); err != nil {
				log.Error(err, "failed to remove finalizer to DNSEntry", "entry", entry.Name)
				return reconcile.Result{}, err
			}
		}
	}

	// TODO: implement logic
	return reconcile.Result{}, nil
}

func (r *Reconciler) findBestMatchingProvider(ctx context.Context, dnsName string, currentProviderName *string) (*v1alpha1.DNSProvider, error) {
	providerList := &v1alpha1.DNSProviderList{}
	if err := r.Client.List(ctx, providerList, client.InNamespace(r.Namespace)); err != nil {
		return nil, err
	}
	return findBestMatchingProvider(dns.FilterProvidersByClass(providerList.Items, r.Class), dnsName, currentProviderName)
}

func (r *Reconciler) getZoneForProvider(provider *v1alpha1.DNSProvider, dnsName string) (*string, *reconcile.Result, error) {
	pstate := r.state.GetProviderState(client.ObjectKeyFromObject(provider))
	if pstate == nil {
		return nil, &reconcile.Result{Requeue: true}, nil // Provider state not yet available, requeue to wait for its reconciliation
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
	if bestMatch == 0 {
		return nil, nil, fmt.Errorf("no matching zone found for DNS name %q in provider %q", dnsName, provider.Name)
	}
	return ptr.To(bestZone.ZoneID().ID), nil, nil
}

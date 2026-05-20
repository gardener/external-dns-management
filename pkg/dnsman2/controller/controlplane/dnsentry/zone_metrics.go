// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/metrics"
)

// zoneMetricsReporter periodically lists all DNSEntry resources and updates the
// per-zone DNS entry count gauge, grouped by status.state.
type zoneMetricsReporter struct {
	client           client.Client
	log              logr.Logger
	namespace        string
	class            string
	secondaryClasses []string
	interval         time.Duration

	previous map[metrics.EntryStateLabels]time.Time
}

func newZoneMetricsReporter(c client.Client, log logr.Logger, namespace, class string, secondaryClasses []string, interval time.Duration) *zoneMetricsReporter {
	return &zoneMetricsReporter{
		client:           c,
		log:              log,
		namespace:        namespace,
		class:            class,
		secondaryClasses: secondaryClasses,
		interval:         interval,
		previous:         map[metrics.EntryStateLabels]time.Time{},
	}
}

// Start implements manager.Runnable.
func (r *zoneMetricsReporter) Start(ctx context.Context) error {
	r.log.Info("Starting zone metrics reporter", "interval", r.interval)
	r.report(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			r.report(ctx)
		}
	}
}

func (r *zoneMetricsReporter) report(ctx context.Context) {
	entryList := &v1alpha1.DNSEntryList{}
	if err := r.client.List(ctx, entryList, client.InNamespace(r.namespace)); err != nil {
		r.log.Error(err, "failed to list DNSEntry resources for zone metrics")
		return
	}
	counts := map[metrics.EntryStateLabels]int{}
	for _, entry := range dns.FilterEntriesByClass(entryList.Items, r.class, r.secondaryClasses) {
		counts[metrics.EntryStateLabels{
			ProviderType: ptr.Deref(entry.Status.ProviderType, ""),
			Zone:         ptr.Deref(entry.Status.Zone, ""),
			State:        entry.Status.State,
		}]++
	}
	r.previous = metrics.ReportEntryStates(counts, r.previous)
}

var _ manager.Runnable = &zoneMetricsReporter{}

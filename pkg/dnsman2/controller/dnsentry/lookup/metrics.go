// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lookup

import (
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/metrics"
)

// LookupMetrics defines metrics reporting for DNS lookup processing.
type LookupMetrics interface {
	// IncrSkipped increments the skipped lookups counter.
	IncrSkipped()
	// IncrHostnameLookups records a hostname lookup event.
	IncrHostnameLookups(name client.ObjectKey, hosts, errorCount int, duration time.Duration)
	// ReportCurrentJobCount reports the current number of lookup jobs.
	ReportCurrentJobCount(count int)
	// IncrLookupChanged increments the counter for changed lookups.
	IncrLookupChanged(name client.ObjectKey)
	// RemoveJob records the removal of a lookup job.
	RemoveJob(name client.ObjectKey)
}

type defaultLookupMetrics struct{}

var _ LookupMetrics = &defaultLookupMetrics{}

func (d defaultLookupMetrics) IncrSkipped() {
	metrics.ReportLookupProcessorIncrSkipped()
}

func (d defaultLookupMetrics) IncrHostnameLookups(key client.ObjectKey, hosts, errorCount int, duration time.Duration) {
	metrics.ReportLookupProcessorIncrHostnameLookups(key, hosts, errorCount, duration)
}

func (d defaultLookupMetrics) ReportCurrentJobCount(count int) {
	metrics.ReportLookupProcessorJobs(count)
}

func (d defaultLookupMetrics) IncrLookupChanged(name client.ObjectKey) {
	metrics.ReportLookupProcessorIncrLookupChanged(name)
}

func (d defaultLookupMetrics) RemoveJob(name client.ObjectKey) {
	metrics.ReportRemovedJob(name)
}

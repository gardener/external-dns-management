// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// RegisterAll registers all metrics with the controller-runtime registry, which is served by the metrics server.
func RegisterAll() {
	ctrlmetrics.Registry.MustRegister(Requests)
	ctrlmetrics.Registry.MustRegister(ZoneRequests)
	ctrlmetrics.Registry.MustRegister(Accounts)
	ctrlmetrics.Registry.MustRegister(Entries)
	ctrlmetrics.Registry.MustRegister(LookupProcessorJobs)
	ctrlmetrics.Registry.MustRegister(LookupProcessorSkips)
	ctrlmetrics.Registry.MustRegister(LookupProcessorLookups)
	ctrlmetrics.Registry.MustRegister(LookupProcessorHosts)
	ctrlmetrics.Registry.MustRegister(LookupProcessorErrors)
	ctrlmetrics.Registry.MustRegister(LookupProcessorLookupChanged)
	ctrlmetrics.Registry.MustRegister(LookupProcessorSeconds)
}

var (
	// Requests tracks the total number of requests per provider type and account.
	Requests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_total_provider_requests",
			Help: "Total requests per provider type and credential set",
		},
		[]string{"providertype", "accounthash", "requesttype"},
	)

	// ZoneRequests tracks the total number of requests per provider type, account, request type, and zone.
	ZoneRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_requests_per_zone",
			Help: "Requests per provider type, credential set, and zone",
		},
		[]string{"providertype", "accounthash", "requesttype", "zone"},
	)

	// Accounts tracks the number of providers per account.
	Accounts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "external_dns_management_account_providers",
			Help: "Total number of providers per account",
		},
		[]string{"providertype", "accounthash"},
	)

	// Entries tracks the number of DNS entries per hosted zone and reconciliation state.
	Entries = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "external_dns_management_dns_entries",
			Help: "Number of DNS entries per hosted zone, grouped by status state",
		},
		[]string{"providertype", "zone", "state"},
	)

	// LookupProcessorJobs tracks the number of jobs in the lookup processor.
	LookupProcessorJobs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "external_dns_management_lookup_processor_jobs",
			Help: "Number of jobs in the lookup processor",
		},
	)

	// LookupProcessorSkips counts the number of skipped lookups due to overload.
	LookupProcessorSkips = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "external_dns_management_lookup_processor_skips",
			Help: "Number of skipped lookups because of overload",
		},
	)

	// LookupProcessorLookups counts the number of lookups per object.
	LookupProcessorLookups = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_lookup_processor_lookups",
			Help: "Number of lookups per object",
		},
		[]string{"namespace"},
	)

	// LookupProcessorLookupChanged counts the number of lookup results that have changed per object.
	LookupProcessorLookupChanged = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_lookup_processor_lookup_changed",
			Help: "Number of lookup results have changed per object",
		},
		[]string{"namespace"},
	)

	// LookupProcessorHosts counts the number of hosts looked up per object.
	LookupProcessorHosts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_lookup_processor_hosts",
			Help: "Number of hosts lookup per object",
		},
		[]string{"namespace"},
	)

	// LookupProcessorErrors counts the number of errors during host lookups per object.
	LookupProcessorErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_lookup_processor_errors",
			Help: "Number of failed host lookups per object",
		},
		[]string{"namespace"},
	)

	// LookupProcessorSeconds measures the duration of lookups in seconds.
	LookupProcessorSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "external_dns_management_lookup_processor_seconds",
			Help:    "Lookup duration of lookup in seconds",
			Buckets: []float64{.01, .02, .05, .1, .2, .5, 1, 2, 5, 10, 20},
		},
	)
)

var theRequestLabels = &requestLabels{lock: sync.Mutex{}, known: map[ptypeAccount]sets.Set[string]{}}

type ptypeAccount struct {
	ptype   string
	account string
}

type requestLabels struct {
	lock  sync.Mutex
	known map[ptypeAccount]sets.Set[string]
}

func (this *requestLabels) AddRequestLabel(ptype, account, requestType string) {
	this.lock.Lock()
	defer this.lock.Unlock()

	key := ptypeAccount{ptype, account}
	set, ok := this.known[key]
	if !ok {
		set = sets.Set[string]{}
		this.known[key] = set
	}
	set.Insert(requestType)
}

func (this *requestLabels) Delete(ptype, account string) sets.Set[string] {
	this.lock.Lock()
	defer this.lock.Unlock()

	key := ptypeAccount{ptype, account}
	set := this.known[key]
	delete(this.known, key)
	return set
}

// DeleteAccount removes all metrics for a given provider type and account.
func DeleteAccount(ptype, account string) {
	Accounts.DeleteLabelValues(ptype, account)
	requestTypes := theRequestLabels.Delete(ptype, account)
	for rtype := range requestTypes {
		Requests.DeleteLabelValues(ptype, account, rtype)
	}
}

// ReportAccountProviders sets the number of providers for a given provider type and account.
func ReportAccountProviders(ptype, account string, amount int) {
	Accounts.WithLabelValues(ptype, account).Set(float64(amount))
}

// AddRequests adds the given number of requests for a provider type, account, request type, and optionally a zone.
func AddRequests(ptype, account, requestType string, no int, zone *string) {
	theRequestLabels.AddRequestLabel(ptype, account, requestType)
	Requests.WithLabelValues(ptype, account, requestType).Add(float64(no))
	if zone != nil {
		ZoneRequests.WithLabelValues(ptype, account, requestType, *zone).Add(float64(no))
	}
}

// EntryStateLabels identifies a unique combination of providertype, zone and entry state used by Entries.
type EntryStateLabels struct {
	ProviderType string
	Zone         string
	State        string
}

// entryStateDeletionGracePeriod is the time a zero-valued label combination is kept
// before it is actually deleted. Prometheus caches scraped series for a short time,
// so deleting immediately would hide that the value dropped to zero.
const entryStateDeletionGracePeriod = 5 * time.Minute

// ReportEntryStates replaces the current values of the Entries gauge with the given counts.
// Label combinations that were present in previous but are missing from counts are set to 0
// and only deleted after they have stayed at 0 for at least entryStateDeletionGracePeriod.
// The returned map must be passed back as previous on the next call.
func ReportEntryStates(counts map[EntryStateLabels]int, previous map[EntryStateLabels]time.Time) map[EntryStateLabels]time.Time {
	now := time.Now()
	current := make(map[EntryStateLabels]time.Time, len(counts)+len(previous))
	for k, v := range counts {
		Entries.WithLabelValues(k.ProviderType, k.Zone, k.State).Set(float64(v))
		current[k] = time.Time{} // not zero / not pending deletion
	}
	for k, zeroSince := range previous {
		if _, ok := counts[k]; ok {
			continue
		}
		if zeroSince.IsZero() {
			// First time we see this combination missing: set to 0 and start the grace period.
			Entries.WithLabelValues(k.ProviderType, k.Zone, k.State).Set(0)
			current[k] = now
			continue
		}
		if now.Sub(zeroSince) >= entryStateDeletionGracePeriod {
			Entries.DeleteLabelValues(k.ProviderType, k.Zone, k.State)
			continue
		}
		// Still within the grace period; keep the original timestamp and ensure value is 0.
		Entries.WithLabelValues(k.ProviderType, k.Zone, k.State).Set(0)
		current[k] = zeroSince
	}
	return current
}

// ReportLookupProcessorIncrSkipped increments the skipped lookups metric.
func ReportLookupProcessorIncrSkipped() {
	LookupProcessorSkips.Inc()
}

// ReportLookupProcessorIncrHostnameLookups reports metrics for hostname lookups.
func ReportLookupProcessorIncrHostnameLookups(name client.ObjectKey, hosts, errorCount int, duration time.Duration) {
	addLookupName(name)
	LookupProcessorLookups.WithLabelValues(name.Namespace).Inc()
	LookupProcessorHosts.WithLabelValues(name.Namespace).Add(float64(hosts))
	LookupProcessorErrors.WithLabelValues(name.Namespace).Add(float64(errorCount))
	LookupProcessorSeconds.Observe(duration.Seconds())
}

// ReportLookupProcessorJobs sets the number of jobs in the lookup processor.
func ReportLookupProcessorJobs(jobs int) {
	LookupProcessorJobs.Set(float64(jobs))
}

// ReportLookupProcessorIncrLookupChanged increments the lookup changed metric for a given object.
func ReportLookupProcessorIncrLookupChanged(name client.ObjectKey) {
	addLookupName(name)
	LookupProcessorLookupChanged.WithLabelValues(name.Namespace).Inc()
}

// ReportRemovedJob removes metrics for a given object key if it was the last in the namespace.
func ReportRemovedJob(name client.ObjectKey) {
	if removeLookupName(name) {
		LookupProcessorLookups.DeleteLabelValues(name.Namespace)
		LookupProcessorHosts.DeleteLabelValues(name.Namespace)
		LookupProcessorErrors.DeleteLabelValues(name.Namespace)
		LookupProcessorLookupChanged.DeleteLabelValues(name.Namespace)
	}
}

var knownLookupNames = sets.New[client.ObjectKey]()
var knownLookupNamesLook sync.Mutex

func addLookupName(name client.ObjectKey) {
	knownLookupNamesLook.Lock()
	defer knownLookupNamesLook.Unlock()
	knownLookupNames.Insert(name)
}

// removeLookupName removes name from known lookup entries and returns true if it was the last in the namespace.
func removeLookupName(name client.ObjectKey) bool {
	knownLookupNamesLook.Lock()
	defer knownLookupNamesLook.Unlock()
	knownLookupNames.Delete(name)
	for n := range knownLookupNames {
		if n.Namespace == name.Namespace {
			return false
		}
	}
	return true
}

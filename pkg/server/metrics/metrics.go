// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"strconv"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/server"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/external-dns-management/pkg/dns"
)

func init() {
	prometheus.MustRegister(Requests)
	prometheus.MustRegister(ZoneRequests)
	prometheus.MustRegister(ZoneCacheDiscardings)
	prometheus.MustRegister(Accounts)
	prometheus.MustRegister(Entries)
	prometheus.MustRegister(StaleEntries)
	prometheus.MustRegister(RemoteAccessLogins)
	prometheus.MustRegister(RemoteAccessRequests)
	prometheus.MustRegister(RemoteAccessSeconds)
	prometheus.MustRegister(RemoteAccessCertificates)
	prometheus.MustRegister(LookupProcessorJobs)
	prometheus.MustRegister(LookupProcessorSkips)
	prometheus.MustRegister(LookupProcessorLookups)
	prometheus.MustRegister(LookupProcessorHosts)
	prometheus.MustRegister(LookupProcessorErrors)
	prometheus.MustRegister(LookupProcessorLookupChanged)
	prometheus.MustRegister(LookupProcessorSeconds)
	prometheus.MustRegister(EntryReconciliations)
	prometheus.MustRegister(ZoneReconciliations)
	prometheus.MustRegister(CompletedZoneReconciliationSeconds)

	server.RegisterHandler("/metrics", promhttp.Handler())
}

var (
	Requests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_total_provider_requests",
			Help: "Total requests per provider type and credential set",
		},
		[]string{"providertype", "accounthash", "requesttype"},
	)

	ZoneRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_requests_per_zone",
			Help: "Requests per provider type, credential set, and zone",
		},
		[]string{"providertype", "accounthash", "requesttype", "zone"},
	)

	ZoneCacheDiscardings = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_zone_cache_discardings",
			Help: "Discardings of zone cache per provider type and zone",
		},
		[]string{"providertype", "zone"},
	)

	Accounts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "external_dns_management_account_providers",
			Help: "Total number of providers per account",
		},
		[]string{"providertype", "accounthash"},
	)

	Entries = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "external_dns_management_dns_entries",
			Help: "Total number of dns entries per hosted zone",
		},
		[]string{"providertype", "zone"},
	)

	StaleEntries = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "external_dns_management_dns_entries_stale",
			Help: "Total number of stale dns entries per hosted zone",
		},
		[]string{"providertype", "zone"},
	)

	RemoteAccessLogins = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_remoteaccess_logins",
			Help: "Total number of remote access logins",
		},
		[]string{"handler", "client", "success"},
	)

	RemoteAccessRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_remoteaccess_requests",
			Help: "Total number of remote access requests",
		},
		[]string{"handler", "client", "type", "zoneid"},
	)

	RemoteAccessSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "external_dns_management_remoteaccess_seconds",
			Help:    "Duration in seconds of completed remote access requests",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 25},
		},
		[]string{"handler", "client", "type", "zoneid", "error"},
	)

	RemoteAccessCertificates = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "external_dns_management_remoteaccess_transport_credentials",
			Help: "Number of server-side transport credentials of remote access",
		},
	)

	LookupProcessorJobs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "external_dns_management_lookup_processor_jobs",
			Help: "Number of jobs in the lookup processor",
		},
	)

	LookupProcessorSkips = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "external_dns_management_lookup_processor_skips",
			Help: "Number of skipped lookups because of overload",
		},
	)

	LookupProcessorLookups = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_lookup_processor_lookups",
			Help: "Number of lookups per object",
		},
		[]string{"namespace"},
	)

	LookupProcessorLookupChanged = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_lookup_processor_lookup_changed",
			Help: "Number of lookup results have changed per object",
		},
		[]string{"namespace"},
	)

	LookupProcessorHosts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_lookup_processor_hosts",
			Help: "Number of hosts lookup per object",
		},
		[]string{"namespace"},
	)

	LookupProcessorErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_lookup_processor_errors",
			Help: "Number of failed host lookups per object",
		},
		[]string{"namespace"},
	)

	LookupProcessorSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "external_dns_management_lookup_processor_seconds",
			Help:    "Lookup duration of lookup in seconds",
			Buckets: []float64{.01, .02, .05, .1, .2, .5, 1, 2, 5, 10, 20},
		},
	)

	EntryReconciliations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_entry_reconciliations_total",
			Help: "Total number of DNS entry reconciliations per provider type and zone",
		},
		[]string{"providertype", "zone"},
	)

	ZoneReconciliations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "external_dns_management_zone_reconciliations_total",
			Help: "Total number of zone reconciliations per provider type, zone and completion status",
		},
		[]string{"providertype", "zone", "completionstatus"},
	)

	CompletedZoneReconciliationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "external_dns_management_zone_reconciliations_seconds",
			Help:    "Completed zone reconciliation in seconds",
			Buckets: []float64{.01, .02, .05, .1, .2, .5, 1, 2, 5, 10, 20},
		},
		[]string{"providertype", "zone"},
	)
)

var theRequestLabels = &requestLabels{lock: sync.Mutex{}, known: map[ptypeAccount]utils.StringSet{}}

type ptypeAccount struct {
	ptype   string
	account string
}

type requestLabels struct {
	lock  sync.Mutex
	known map[ptypeAccount]utils.StringSet
}

func (this *requestLabels) AddRequestLabel(ptype, account, requestType string) {
	this.lock.Lock()
	defer this.lock.Unlock()

	key := ptypeAccount{ptype, account}
	set, ok := this.known[key]
	if !ok {
		set = utils.StringSet{}
		this.known[key] = set
	}
	set.Add(requestType)
}

func (this *requestLabels) Delete(ptype, account string) utils.StringSet {
	this.lock.Lock()
	defer this.lock.Unlock()

	key := ptypeAccount{ptype, account}
	set := this.known[key]
	delete(this.known, key)
	return set
}

func DeleteAccount(ptype, account string) {
	Accounts.DeleteLabelValues(ptype, account)
	requestTypes := theRequestLabels.Delete(ptype, account)
	for rtype := range requestTypes {
		Requests.DeleteLabelValues(ptype, account, rtype)
	}
	Entries.DeleteLabelValues(ptype, account)
}

func ReportAccountProviders(ptype, account string, amount int) {
	Accounts.WithLabelValues(ptype, account).Set(float64(amount))
}

func AddRequests(ptype, account, requestType string, no int, zone *string) {
	theRequestLabels.AddRequestLabel(ptype, account, requestType)
	Requests.WithLabelValues(ptype, account, requestType).Add(float64(no))
	if zone != nil {
		ZoneRequests.WithLabelValues(ptype, account, requestType, *zone).Add(float64(no))
	}
}

func AddZoneCacheDiscarding(id dns.ZoneID) {
	ZoneCacheDiscardings.WithLabelValues(id.ProviderType, id.ID).Add(float64(1))
}

type ZoneProviderTypes struct {
	lock      sync.Mutex
	providers map[dns.ZoneID]struct{}
}

func (this *ZoneProviderTypes) Add(zone dns.ZoneID) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.providers[zone] = struct{}{}
}

func (this *ZoneProviderTypes) Remove(zone dns.ZoneID) {
	this.lock.Lock()
	defer this.lock.Unlock()
	delete(this.providers, zone)
}

var zoneProviders = &ZoneProviderTypes{providers: map[dns.ZoneID]struct{}{}}

func ReportZoneEntries(zoneid dns.ZoneID, amount int, stale int) {
	Entries.WithLabelValues(zoneid.ProviderType, zoneid.ID).Set(float64(amount))
	StaleEntries.WithLabelValues(zoneid.ProviderType, zoneid.ID).Set(float64(stale))
	zoneProviders.Add(zoneid)
}

func ReportRemoteAccessLogins(namespace, client string, success bool) {
	RemoteAccessLogins.WithLabelValues(namespace, client, strconv.FormatBool(success)).Add(float64(1))
}

func ReportRemoteAccessRequests(namespace, client, requestType, zoneid string) {
	RemoteAccessRequests.WithLabelValues(namespace, client, requestType, zoneid).Add(float64(1))
}

func ReportRemoteAccessSeconds(namespace, client, requestType, zoneid, error string, duration time.Duration) {
	RemoteAccessSeconds.WithLabelValues(namespace, client, requestType, zoneid, error).Observe(duration.Seconds())
}

func ReportRemoteAccessCertificates(count int) {
	RemoteAccessCertificates.Set(float64(count))
}

func DeleteZone(zoneid dns.ZoneID) {
	zoneProviders.Remove(zoneid)
	Entries.DeleteLabelValues(zoneid.ProviderType, zoneid.ID)
}

func ReportLookupProcessorIncrSkipped() {
	LookupProcessorSkips.Inc()
}

func ReportLookupProcessorIncrHostnameLookups(name resources.ObjectName, hosts, errorCount int, duration time.Duration) {
	addLookupName(name)
	LookupProcessorLookups.WithLabelValues(name.Namespace()).Inc()
	LookupProcessorHosts.WithLabelValues(name.Namespace()).Add(float64(hosts))
	LookupProcessorErrors.WithLabelValues(name.Namespace()).Add(float64(errorCount))
	LookupProcessorSeconds.Observe(duration.Seconds())
}

func ReportLookupProcessorJobs(jobs int) {
	LookupProcessorJobs.Set(float64(jobs))
}

func ReportLookupProcessorIncrLookupChanged(name resources.ObjectName) {
	addLookupName(name)
	LookupProcessorLookupChanged.WithLabelValues(name.Namespace()).Inc()
}

func ReportRemovedJob(name resources.ObjectName) {
	if removeLookupName(name) {
		LookupProcessorLookups.DeleteLabelValues(name.Namespace())
		LookupProcessorHosts.DeleteLabelValues(name.Namespace())
		LookupProcessorErrors.DeleteLabelValues(name.Namespace())
		LookupProcessorLookupChanged.DeleteLabelValues(name.Namespace())
	}
}

func ReportCompletedZoneReconciliation(ptype, zone string, duration time.Duration) {
	CompletedZoneReconciliationSeconds.WithLabelValues(ptype, zone).Observe(duration.Seconds())
	ZoneReconciliations.WithLabelValues(ptype, zone, "success").Inc()
}

func ReportNotCompletedZoneReconciliation(ptype, zone, reason string) {
	ZoneReconciliations.WithLabelValues(ptype, zone, reason).Inc()
}

func ReportEntryReconciliation(ptype, zone string) {
	EntryReconciliations.WithLabelValues(ptype, zone).Inc()
}

var knownLookupNames = sets.New[resources.ObjectName]()
var knownLookupNamesLook sync.Mutex

func addLookupName(name resources.ObjectName) {
	knownLookupNamesLook.Lock()
	defer knownLookupNamesLook.Unlock()
	knownLookupNames.Insert(name)
}

// removeLookupName removes name from known lookup entries and returns true if it was the last in the namespace.
func removeLookupName(name resources.ObjectName) bool {
	knownLookupNamesLook.Lock()
	defer knownLookupNamesLook.Unlock()
	knownLookupNames.Delete(name)
	for n := range knownLookupNames {
		if n.Namespace() == name.Namespace() {
			return false
		}
	}
	return true
}

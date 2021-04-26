/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package metrics

import (
	"sync"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/server"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/gardener/external-dns-management/pkg/dns/provider/statistic"
)

func init() {
	prometheus.MustRegister(Requests)
	prometheus.MustRegister(ZoneRequests)
	prometheus.MustRegister(ZoneCacheDiscardings)
	prometheus.MustRegister(Accounts)
	prometheus.MustRegister(Entries)
	prometheus.MustRegister(StaleEntries)
	prometheus.MustRegister(Owners)

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

	Owners = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "external_dns_management_dns_owners",
			Help: "Total number of dns entries per owner",
		},
		[]string{"owner", "providertype", "provider"},
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

func AddZoneCacheDiscarding(ptype, zone string) {
	ZoneCacheDiscardings.WithLabelValues(ptype, zone).Add(float64(1))
}

type ZoneProviderTypes struct {
	lock      sync.Mutex
	providers map[string]string
}

func (this *ZoneProviderTypes) Add(ptype, zone string) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.providers[zone] = ptype
}

func (this *ZoneProviderTypes) Remove(zone string) string {
	this.lock.Lock()
	defer this.lock.Unlock()
	ptype := this.providers[zone]
	delete(this.providers, zone)
	return ptype
}

var zoneProviders = &ZoneProviderTypes{providers: map[string]string{}}

func ReportZoneEntries(ptype, zone string, amount int, stale int) {
	Entries.WithLabelValues(ptype, zone).Set(float64(amount))
	StaleEntries.WithLabelValues(ptype, zone).Set(float64(stale))
	zoneProviders.Add(ptype, zone)
}

func DeleteZone(zone string) {
	ptype := zoneProviders.Remove(zone)
	if ptype != "" {
		Entries.DeleteLabelValues(ptype, zone)
	}
}

var currentStatistic = statistic.NewEntryStatistic()
var lock sync.Mutex

func deleteOwnerStatistic(state statistic.WalkingState, owner, ptype string, pname resources.ObjectName, count int) statistic.WalkingState {
	types := state.(utils.StringSet)
	if types.Contains(ptype) {
		Owners.DeleteLabelValues(owner, ptype, pname.String())
	}
	return state
}

func UpdateOwnerStatistic(statistic *statistic.EntryStatistic, types utils.StringSet) {
	lock.Lock()
	defer lock.Unlock()

	for o := range currentStatistic.Owners {
		statistic.Owners.Assure(o)
	}
	for o, pts := range statistic.Owners {
		old_pts := currentStatistic.Owners.Assure(o)
		for pt := range types {
			ps := pts.Get(pt)
			old_ps := old_pts.Assure(pt)
			for p, c := range ps {
				Owners.WithLabelValues(o, pt, p.String()).Set(float64(c))
				old_ps[p] = c
			}
			for p := range old_ps {
				if _, ok := ps[p]; !ok {
					Owners.DeleteLabelValues(o, pt, p.String())
					delete(old_ps, p)
				}
			}
			if len(old_ps) == 0 {
				delete(old_pts, pt)
			}
		}
		for pt, ps := range old_pts {
			if pts[pt] == nil && types.Contains(pt) {
				ps.Walk(types, deleteOwnerStatistic, o, pt)
				delete(old_pts, pt)
			}
		}
		if len(old_pts) == 0 {
			delete(currentStatistic.Owners, o)
		}
	}
}

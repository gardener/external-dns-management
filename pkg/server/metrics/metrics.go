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
	"github.com/gardener/controller-manager-library/pkg/server"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func init() {
	prometheus.MustRegister(Requests)
	prometheus.MustRegister(Accounts)
	prometheus.MustRegister(Entries)

	server.RegisterHandler("/metrics", promhttp.Handler())
}

var (
	Requests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "total_provider_requests",
			Help: "Total requests per provider type and credential set",
		},
		[]string{"providertype", "accounthash"},
	)

	Accounts = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "account_providers",
			Help: "Total number of providers per account",
		},
		[]string{"providertype", "accounthash"},
	)

	Entries = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dns_entries",
			Help: "Total number of dns entries per account",
		},
		[]string{"providertype", "accounthash"},
	)

)

func DeleteAccount(ptype, account string) {
	Requests.DeleteLabelValues(ptype, account)
	Accounts.DeleteLabelValues(ptype, account)
	Entries.DeleteLabelValues(ptype, account)
}

func ReportAccountProviders(ptype, account string, amount int) {
	Accounts.WithLabelValues(ptype, account).Set(float64(amount))
}

func AddRequests(ptype, account string, no int) {
	Requests.WithLabelValues(ptype, account).Add(float64(no))
}

func ReportAccountEntries(ptype, account string, amount int) {
	Entries.WithLabelValues(ptype, account).Set(float64(amount))
}

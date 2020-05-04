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

package controller

import (
	"github.com/gardener/external-dns-management/pkg/controller/provider/openstack"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

func init() {
	provider.DNSController("", openstack.Factory).
		FinalizerDomain("dns.gardener.cloud").
		DefaultedBoolOption(provider.OPT_RATELIMITER_ENABLED, true, "enables rate limiter for Openstack Designate requests").
		DefaultedIntOption(provider.OPT_RATELIMITER_QPS, 100, "maximum requests/queries per second").
		DefaultedIntOption(provider.OPT_RATELIMITER_BURST, 20, "number of burst requests for rate limiter").
		MustRegister(provider.CONTROLLER_GROUP_DNS_CONTROLLERS)
}

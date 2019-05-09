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

package provider

import (
	"github.com/gardener/external-dns-management/pkg/dns/source"
)

/*
  Standard options a DNS Reconciler should offer
*/

const (
	OPT_IDENTIFIER      = "identifier"
	OPT_CLASS           = source.OPT_CLASS
	OPT_DRYRUN          = "dry-run"
	OPT_TTL             = "ttl"
	OPT_CACHE_TTL       = "cache-ttl"
	OPT_SETUP           = "setup"
	OPT_DNSDELAY        = "dns-delay"
	OPT_RESCHEDULEDELAY = "reschedule-delay"

	OPT_PROVIDERTYPES = "provider-types"

	HOSTEDZONE_PREFIX = "hostedzone:"
)

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
 * limitations under the License.
 *
 */

package dns

const (
	CONTROLLER_GROUP_DNS_CONTROLLERS = "dnscontrollers"
	CONTROLLER_GROUP_DNS_SOURCES     = "dnssources"
	CONTROLLER_GROUP_REPLICATION     = "replication"
)

const (
	DEFAULT_CLASS               = "gardendns"
	ANNOTATION_GROUP            = "dns.gardener.cloud"
	CLASS_ANNOTATION            = ANNOTATION_GROUP + "/class"
	REALM_ANNOTATION            = ANNOTATION_GROUP + "/realms"
	NOT_RATE_LIMITED_ANNOTATION = ANNOTATION_GROUP + "/not-rate-limited"
)

const OPT_SETUP = "setup"

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
	"fmt"
	"net"

	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type (
	Target  = dnsutils.Target
	Targets = dnsutils.Targets
)

func NewHostTargetFromEntryVersion(name string, entry *EntryVersion) (Target, error) {
	ip := net.ParseIP(name)
	if ip == nil {
		return dnsutils.NewTarget(dns.RS_CNAME, name, entry.TTL()), nil
	} else if ip.To4() != nil {
		return dnsutils.NewTarget(dns.RS_A, name, entry.TTL()), nil
	} else if ip.To16() != nil {
		return dnsutils.NewTarget(dns.RS_AAAA, name, entry.TTL()), nil
	} else {
		return nil, fmt.Errorf("unexpected IP address (never ipv4 or ipv6): %s (%s)", ip.String(), name)
	}
}

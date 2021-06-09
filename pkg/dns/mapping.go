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

package dns

import (
	"strings"
)

////////////////////////////////////////////////////////////////////////////////
// Text Record ObjectName Mapping
////////////////////////////////////////////////////////////////////////////////

var TxtPrefix = "comment-"

func AlignHostname(host string) string {
	if strings.HasSuffix(host, ".") {
		return host
	}
	return host + "."
}

func NormalizeHostname(host string) string {
	if strings.HasPrefix(host, "\\052.") {
		host = "*" + host[4:]
	}
	if strings.HasSuffix(host, ".") {
		return host[:len(host)-1]
	}
	return host
}

func MapToProvider(rtype string, dnsset *DNSSet, base string) (string, *RecordSet) {
	name := dnsset.Name
	rs := dnsset.Sets[rtype]
	if rtype == RS_META {
		prefix := dnsset.GetMetaAttr(ATTR_PREFIX)
		if prefix == "" {
			prefix = TxtPrefix
			dnsset.SetMetaAttr(ATTR_PREFIX, prefix)
		}
		new := *dnsset.Sets[rtype]
		new.Type = RS_TXT
		add := ""
		if strings.HasPrefix(name, "*.") {
			add = "*."
			name = name[2:]
			if name == base {
				prefix += "-base."
			}
		}
		return add + prefix + name, &new
	}
	return name, rs
}

func MapFromProvider(dns string, rs *RecordSet) (string, *RecordSet) {
	if rs.Type == RS_TXT {
		prefix := rs.GetAttr(ATTR_PREFIX)
		if prefix != "" {
			add := ""
			if strings.HasPrefix(dns, "*.") {
				add = "*."
				dns = dns[2:]
			}
			if strings.HasPrefix(dns, prefix) {
				new := *rs
				new.Type = RS_META
				dns = dns[len(prefix):]
				if strings.HasPrefix(dns, "-base.") {
					dns = dns[6:]
				} else if strings.HasPrefix(dns, ".") {
					// for backwards compatibility of form *.comment-.basedomain
					dns = dns[1:]
				}
				return add + dns, &new
			} else {
				return add + dns, rs
			}
		}
	}
	return dns, rs
}

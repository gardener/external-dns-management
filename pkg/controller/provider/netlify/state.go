/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package netlify

import (
	"github.com/netlify/open-api/go/models"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type Record models.DNSRecord

func (r *Record) GetType() string    { return r.Type }
func (r *Record) GetId() string      { return r.ID }
func (r *Record) GetDNSName() string { return r.Hostname }
func (r *Record) GetValue() string {
	if r.Type == dns.RS_TXT {
		return raw.EnsureQuotedText(r.Value)
	}
	return r.Value
}
func (r *Record) GetTTL() int      { return int(r.TTL) } // The Netlify Record uses int64 for TTL
func (r *Record) SetTTL(ttl int)   { r.TTL = int64(ttl) } // The Netlify Record uses int64 for TTL
func (r *Record) Copy() raw.Record { n := *r; return &n }

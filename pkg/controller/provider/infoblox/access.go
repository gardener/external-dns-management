/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package infoblox

import (
	"strconv"
	"strings"

	ibclient "github.com/infobloxopen/infoblox-go-client/v2"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type access struct {
	ibclient.IBConnector
	metrics provider.Metrics
	view    string
}

var _ raw.Executor = (*access)(nil)

func NewAccess(client ibclient.IBConnector, view string, metrics provider.Metrics) *access {
	return &access{
		IBConnector: client,
		metrics:     metrics,
		view:        view,
	}
}

func (this *access) CreateRecord(r raw.Record, zone provider.DNSHostedZone) error {
	this.metrics.AddZoneRequests(zone.Id(), provider.M_CREATERECORDS, 1)
	_, err := this.CreateObject(r.(ibclient.IBObject))
	return err
}

func (this *access) UpdateRecord(r raw.Record, zone provider.DNSHostedZone) error {
	this.metrics.AddZoneRequests(zone.Id(), provider.M_CREATERECORDS, 1)
	_, err := this.UpdateObject(r.(Record).PrepareUpdate().(ibclient.IBObject), r.GetId())
	return err
}

func (this *access) DeleteRecord(r raw.Record, zone provider.DNSHostedZone) error {
	this.metrics.AddZoneRequests(zone.Id(), provider.M_DELETERECORDS, 1)
	_, err := this.DeleteObject(r.GetId())
	return err
}

func (this *access) NewRecord(fqdn string, rtype string, value string, zone provider.DNSHostedZone, ttl int64) (record raw.Record) {
	switch rtype {
	case dns.RS_A:
		r := ibclient.NewEmptyRecordA()
		r.Name = fqdn
		r.Ipv4Addr = value
		r.View = this.view
		record = (*RecordA)(r)
	case dns.RS_AAAA:
		r := ibclient.NewEmptyRecordAAAA()
		r.Name = fqdn
		r.Ipv6Addr = value
		r.View = this.view
		record = (*RecordAAAA)(r)
	case dns.RS_CNAME:
		r := ibclient.NewEmptyRecordCNAME()
		r.Name = fqdn
		r.Canonical = value
		r.View = this.view
		record = (*RecordCNAME)(r)
	case dns.RS_TXT:
		if n, err := strconv.Unquote(value); err == nil && !strings.Contains(value, " ") {
			value = n
		}
		record = (*RecordTXT)(ibclient.NewRecordTXT(ibclient.RecordTXT{
			Name: fqdn,
			Text: value,
			View: this.view,
		}))
	}
	if record != nil {
		record.SetTTL(int(ttl))
	}
	return
}

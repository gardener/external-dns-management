/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. h file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
	"sync"

	"github.com/gardener/external-dns-management/pkg/dns"
)

type zonedata struct {
	zone    DNSHostedZone
	dnssets dns.DNSSets
}

type InMemory struct {
	lock  sync.Mutex
	zones map[dns.ZoneID]zonedata
}

func NewInMemory() *InMemory {
	return &InMemory{zones: map[dns.ZoneID]zonedata{}}
}

func (m *InMemory) GetZones() DNSHostedZones {
	m.lock.Lock()
	defer m.lock.Unlock()

	zones := DNSHostedZones{}
	for _, z := range m.zones {
		zones = append(zones, z.zone)
	}
	return zones
}

func (m *InMemory) FindHostedZone(zoneid dns.ZoneID) DNSHostedZone {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, ok := m.zones[zoneid]
	if !ok {
		return nil
	}
	return data.zone
}

func (m *InMemory) CloneZoneState(zone DNSHostedZone) (DNSZoneState, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, ok := m.zones[zone.Id()]
	if !ok {
		return nil, fmt.Errorf("DNSZone %s not hosted", zone.Id())
	}

	dnssets := data.dnssets.Clone()
	return NewDNSZoneState(dnssets), nil
}

func (m *InMemory) SetZone(zone DNSHostedZone, zoneState DNSZoneState) {
	clone := zoneState.GetDNSSets().Clone()

	m.lock.Lock()
	defer m.lock.Unlock()
	m.zones[zone.Id()] = zonedata{zone: zone, dnssets: clone}
}

func (m *InMemory) DeleteZone(zone DNSHostedZone) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.zones, zone.Id())
}

func (m *InMemory) AddZone(zone DNSHostedZone) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, ok := m.zones[zone.Id()]
	if ok {
		return false
	}

	m.zones[zone.Id()] = zonedata{zone: zone, dnssets: dns.DNSSets{}}
	return true
}

func (m *InMemory) Apply(zoneID dns.ZoneID, request *ChangeRequest, metrics Metrics) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, ok := m.zones[zoneID]
	if !ok {
		return fmt.Errorf("DNSZone %s not hosted", zoneID)
	}

	name, rset := buildRecordSet(request)

	switch request.Action {
	case R_CREATE, R_UPDATE:
		data.dnssets.AddRecordSet(name, rset)
		metrics.AddZoneRequests(zoneID.ID, M_UPDATERECORDS, 1)
	case R_DELETE:
		data.dnssets.RemoveRecordSet(name, rset.Type)
		metrics.AddZoneRequests(zoneID.ID, M_DELETERECORDS, 1)
	}
	return nil
}

func buildRecordSet(req *ChangeRequest) (string, *dns.RecordSet) {
	var dnsset *dns.DNSSet
	switch req.Action {
	case R_CREATE, R_UPDATE:
		dnsset = req.Addition
	case R_DELETE:
		dnsset = req.Deletion
	}

	return dnsset.Name, dnsset.Sets[req.Type]
}

type DumpDNSHostedZone struct {
	ProviderType     string
	Key              string
	Id               string
	Domain           string
	ForwardedDomains []string
}

type ZoneDump struct {
	HostedZone DumpDNSHostedZone
	DNSSets    dns.DNSSets
}
type FullDump struct {
	InMemory map[dns.ZoneID]*ZoneDump
}

func (m *InMemory) BuildFullDump() *FullDump {
	m.lock.Lock()
	defer m.lock.Unlock()

	all := FullDump{InMemory: map[dns.ZoneID]*ZoneDump{}}

	for zoneId := range m.zones {
		all.InMemory[zoneId] = m.buildZoneDump(zoneId)
	}
	return &all
}

func (m *InMemory) buildZoneDump(zoneId dns.ZoneID) *ZoneDump {
	data, ok := m.zones[zoneId]
	if !ok {
		return nil
	}
	hostedZone := DumpDNSHostedZone{ProviderType: data.zone.Id().ProviderType, Id: data.zone.Id().ID, Domain: data.zone.Domain(),
		Key: data.zone.Key(), ForwardedDomains: data.zone.ForwardedDomains()}

	return &ZoneDump{HostedZone: hostedZone, DNSSets: data.dnssets}
}

/*
func (m *InMemory) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fullDump := m.BuildFullDump()

	js, err := json.Marshal(*fullDump)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}
*/

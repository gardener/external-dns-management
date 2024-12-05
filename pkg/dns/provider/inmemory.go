// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"sync"

	"github.com/gardener/external-dns-management/pkg/dns"
	"k8s.io/apimachinery/pkg/util/uuid"
	yaml "sigs.k8s.io/yaml/goyaml.v3"
)

type zonedata struct {
	zone    DNSHostedZone
	dnssets dns.DNSSets
}

// InMemory is a simple in-memory DNS provider implementation
type InMemory struct {
	lock            sync.Mutex
	zones           map[dns.ZoneID]zonedata
	failSimulations map[string]*inMemoryApplyFailSimulation
}

// inMemoryApplyFailSimulation is a struct to simulate apply failures.
type inMemoryApplyFailSimulation struct {
	zoneID       dns.ZoneID
	request      *ChangeRequest
	appliedCount int
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

func (m *InMemory) DeleteZone(zoneID dns.ZoneID) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.zones, zoneID)
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

	for _, fail := range m.failSimulations {
		if fail.zoneID == zoneID && fail.request.IsSemanticEqualTo(request) {
			fail.appliedCount++
			return fmt.Errorf("simulated failure")
		}
	}

	name, rset := buildRecordSet(request)
	switch request.Action {
	case R_CREATE, R_UPDATE:
		data.dnssets.AddRecordSet(name, request.Addition.RoutingPolicy, rset)
		metrics.AddZoneRequests(zoneID.ID, M_UPDATERECORDS, 1)
	case R_DELETE:
		data.dnssets.RemoveRecordSet(name, rset.Type)
		metrics.AddZoneRequests(zoneID.ID, M_DELETERECORDS, 1)
	}
	return nil
}

func buildRecordSet(req *ChangeRequest) (dns.DNSSetName, *dns.RecordSet) {
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

func (m *InMemory) AddApplyFailSimulation(id dns.ZoneID, request *ChangeRequest) string {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.failSimulations == nil {
		m.failSimulations = map[string]*inMemoryApplyFailSimulation{}
	}
	uid := string(uuid.NewUUID())
	m.failSimulations[uid] = &inMemoryApplyFailSimulation{zoneID: id, request: request}
	return uid
}

func (m *InMemory) GetApplyFailSimulationCount(uid string) int {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.failSimulations == nil {
		return 0
	}
	fail, ok := m.failSimulations[uid]
	if !ok {
		return 0
	}
	return fail.appliedCount
}

func (m *InMemory) RemoveApplyFailSimulation(uid string) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.failSimulations == nil {
		return false
	}
	_, ok := m.failSimulations[uid]
	if ok {
		delete(m.failSimulations, uid)
	}
	return ok
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
	hostedZone := DumpDNSHostedZone{
		ProviderType: data.zone.Id().ProviderType, Id: data.zone.Id().ID, Domain: data.zone.Domain(),
		Key: data.zone.Key(), ForwardedDomains: data.zone.ForwardedDomains(),
	}

	return &ZoneDump{HostedZone: hostedZone, DNSSets: data.dnssets}
}

func (d *FullDump) ToYAMLString() string {
	data, err := yaml.Marshal(d.InMemory)
	if err != nil {
		return "error: " + err.Error()
	}
	return string(data)
}

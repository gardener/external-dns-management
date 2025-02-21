// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mock

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/util/uuid"
	yaml "sigs.k8s.io/yaml/goyaml.v3"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

type zonedata struct {
	zone    provider.DNSHostedZone
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
	request      *provider.ChangeRequest
	appliedCount int
}

func NewInMemory() *InMemory {
	return &InMemory{zones: map[dns.ZoneID]zonedata{}}
}

func (m *InMemory) GetZones() []provider.DNSHostedZone {
	m.lock.Lock()
	defer m.lock.Unlock()

	zones := []provider.DNSHostedZone{}
	for _, z := range m.zones {
		zones = append(zones, z.zone)
	}
	return zones
}

func (m *InMemory) FindHostedZone(zoneid dns.ZoneID) provider.DNSHostedZone {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, ok := m.zones[zoneid]
	if !ok {
		return nil
	}
	return data.zone
}

func (m *InMemory) DeleteZone(zoneID dns.ZoneID) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.zones, zoneID)
}

func (m *InMemory) AddZone(zone provider.DNSHostedZone) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, ok := m.zones[zone.ZoneID()]
	if ok {
		return false
	}

	m.zones[zone.ZoneID()] = zonedata{zone: zone, dnssets: dns.DNSSets{}}
	return true
}

func (m *InMemory) GetRecordset(zoneID dns.ZoneID, name dns.DNSSetName, rtype dns.RecordType) *dns.RecordSet {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, ok := m.zones[zoneID]
	if !ok {
		return nil
	}

	return data.dnssets[name].Sets[rtype].Clone()
}

func (m *InMemory) Apply(zoneID dns.ZoneID, request *provider.ChangeRequest, metrics provider.Metrics) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	return fmt.Errorf("not implemented")
	// TODO
	/*
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

		func buildRecordSet(req *provider.ChangeRequest) (dns.DNSSetName, *dns.RecordSet) {
			var dnsset *dns.DNSSet
			switch req.Action {
			case R_CREATE, R_UPDATE:
				dnsset = req.Addition
			case R_DELETE:
				dnsset = req.Deletion
			}

			return dnsset.Name, dnsset.Sets[req.Type]

	*/
}

type DumpDNSHostedZone struct {
	ProviderType string
	Key          string
	ID           string
	Domain       string
}

type ZoneDump struct {
	HostedZone DumpDNSHostedZone
	DNSSets    dns.DNSSets
}
type FullDump struct {
	InMemory map[dns.ZoneID]*ZoneDump
}

func (m *InMemory) AddApplyFailSimulation(id dns.ZoneID, request *provider.ChangeRequest) string {
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
		ProviderType: data.zone.ZoneID().ProviderType,
		ID:           data.zone.ZoneID().ID,
		Domain:       data.zone.Domain(),
		Key:          data.zone.Key(),
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

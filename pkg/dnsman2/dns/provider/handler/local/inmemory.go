// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package local

import (
	"fmt"
	"reflect"
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

// InMemory is a simple in-memory DNS provider implementation.
type InMemory struct {
	lock                 sync.Mutex
	zones                map[dns.ZoneID]zonedata
	failSimulations      map[string]*inMemoryApplyFailSimulation
	supportRoutingPolicy bool
}

// inMemoryApplyFailSimulation is a struct to simulate apply failures.
type inMemoryApplyFailSimulation struct {
	zoneID       dns.ZoneID
	request      *provider.ChangeRequests
	appliedCount int
}

// NewInMemory creates a new InMemory DNS provider.
func NewInMemory(supportRoutingPolicy bool) *InMemory {
	return &InMemory{
		zones:                map[dns.ZoneID]zonedata{},
		supportRoutingPolicy: supportRoutingPolicy,
	}
}

// GetZones returns all hosted zones in the in-memory provider.
func (m *InMemory) GetZones() []provider.DNSHostedZone {
	m.lock.Lock()
	defer m.lock.Unlock()

	zones := []provider.DNSHostedZone{}
	for _, z := range m.zones {
		zones = append(zones, z.zone)
	}
	return zones
}

// FindHostedZone finds a hosted zone by its ZoneID.
func (m *InMemory) FindHostedZone(zoneid dns.ZoneID) provider.DNSHostedZone {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, ok := m.zones[zoneid]
	if !ok {
		return nil
	}
	return data.zone
}

// DeleteZone deletes a hosted zone by its ZoneID.
func (m *InMemory) DeleteZone(zoneID dns.ZoneID) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.zones, zoneID)
}

// AddZone adds a hosted zone to the in-memory provider.
func (m *InMemory) AddZone(zone provider.DNSHostedZone) bool {
	m.lock.Lock()
	defer m.lock.Unlock()

	_, ok := m.zones[zone.ZoneID()]
	if ok {
		return false
	}

	m.zones[zone.ZoneID()] = zonedata{
		zone:    zone,
		dnssets: dns.DNSSets{},
	}
	return true
}

// GetDNSSets returns the DNS sets for a given zone.
func (m *InMemory) GetDNSSets(zoneID dns.ZoneID) dns.DNSSets {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, ok := m.zones[zoneID]
	if !ok {
		return nil
	}

	return data.dnssets.Clone()
}

// GetCounts returns the number of DNS set names and record sets for a zone.
func (m *InMemory) GetCounts(zoneID dns.ZoneID) (nameCount, recordSetCount int) {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, ok := m.zones[zoneID]
	if !ok {
		return 0, 0
	}

	nameCount = len(data.dnssets)
	recordSetCount = 0
	for _, sets := range data.dnssets {
		recordSetCount += len(sets.Sets)
	}
	return nameCount, recordSetCount
}

// GetRecordset returns a specific record set for a zone, name, and record type.
func (m *InMemory) GetRecordset(zoneID dns.ZoneID, name dns.DNSSetName, rtype dns.RecordType) *dns.RecordSet {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, ok := m.zones[zoneID]
	if !ok {
		return nil
	}

	dnsSets, ok := data.dnssets[name]
	if !ok {
		return nil
	}

	return dnsSets.Sets[rtype].Clone()
}

// Apply applies a DNS change request update to the in-memory provider.
func (m *InMemory) Apply(zoneID dns.ZoneID, name dns.DNSSetName, rtype dns.RecordType, update *provider.ChangeRequestUpdate, metrics provider.Metrics) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	data, ok := m.zones[zoneID]
	if !ok {
		return fmt.Errorf("DNSZone %s not hosted", zoneID)
	}

	for _, fail := range m.failSimulations {
		if fail.zoneID == zoneID && isIncluded(fail.request, name, rtype, update) {
			fail.appliedCount++
			return fmt.Errorf("simulated failure")
		}
	}

	if !m.supportRoutingPolicy {
		if name.SetIdentifier != "" || (update.Old != nil && update.Old.RoutingPolicy != nil) || (update.New != nil && update.New.RoutingPolicy != nil) {
			return fmt.Errorf("in-memory provider does not support routing policies")
		}
	}
	if update.Old != nil {
		data.dnssets.RemoveRecordSet(name, update.Old.Type)
		metrics.AddZoneRequests(zoneID.ID, provider.MetricsRequestTypeDeleteRecords, 1)
	}
	if update.New != nil {
		data.dnssets.AddRecordSet(name, update.New)
		metrics.AddZoneRequests(zoneID.ID, provider.MetricsRequestTypeCreateRecords, 1)
	}
	return nil
}

// DumpDNSHostedZone represents a hosted zone for dumping purposes.
type DumpDNSHostedZone struct {
	ProviderType string
	Key          string
	ID           string
	Domain       string
}

// ZoneDump represents a dump of a hosted zone and its DNS sets.
type ZoneDump struct {
	HostedZone DumpDNSHostedZone
	DNSSets    dns.DNSSets
}

// FullDump represents a dump of all in-memory zones.
type FullDump struct {
	InMemory map[dns.ZoneID]*ZoneDump
}

// AddApplyFailSimulation adds a simulation for apply failures for a zone and request.
func (m *InMemory) AddApplyFailSimulation(id dns.ZoneID, request *provider.ChangeRequests) string {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.failSimulations == nil {
		m.failSimulations = map[string]*inMemoryApplyFailSimulation{}
	}
	uid := string(uuid.NewUUID())
	m.failSimulations[uid] = &inMemoryApplyFailSimulation{zoneID: id, request: request}
	return uid
}

// GetApplyFailSimulationCount returns the number of times a simulated failure has occurred for a given simulation ID.
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

// RemoveApplyFailSimulation removes a simulated apply failure by its ID.
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

// BuildFullDump creates a full dump of all zones and their DNS sets.
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

// ToYAMLString returns the YAML representation of the full dump.
func (d *FullDump) ToYAMLString() string {
	data, err := yaml.Marshal(d.InMemory)
	if err != nil {
		return "error: " + err.Error()
	}
	return string(data)
}

func isIncluded(cr *provider.ChangeRequests, name dns.DNSSetName, recordType dns.RecordType, update *provider.ChangeRequestUpdate) bool {
	if cr.Name != name {
		return false
	}
	return reflect.DeepEqual(cr.Updates[recordType], update)
}

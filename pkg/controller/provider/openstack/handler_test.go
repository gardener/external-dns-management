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

package openstack

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/gardener/controller-manager-library/pkg/logger"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"

	"github.com/gophercloud/gophercloud/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/openstack/dns/v2/zones"
)

type testzone struct {
	zone   *zones.Zone
	rsmap  map[string]map[string]*recordsets.RecordSet
	id2rs  map[string]*recordsets.RecordSet
	nextID int
}

var _ provider.DNSHostedZone = &testzone{}

func (tz *testzone) buildNextId() string {
	d := fmt.Sprintf("rs-%04d", tz.nextID)
	tz.nextID++
	return d
}

func (tz *testzone) Key() string {
	return tz.zone.ID
}

func (tz *testzone) Id() dns.ZoneID {
	return dns.NewZoneID("test", tz.zone.ID)
}

func (tz *testzone) Domain() string {
	return tz.zone.Name
}

func (tz *testzone) ForwardedDomains() []string {
	return []string{} // not implemented
}

func (tz *testzone) IsPrivate() bool {
	return false
}

func (tz *testzone) Match(dnsname string) int {
	return provider.Match(tz, dnsname)
}

type designateMockClient struct {
	tzmap map[string]*testzone
}

var _ designateClientInterface = &designateMockClient{}

var mockMetrics provider.Metrics = &provider.NullMetrics{}

func (c *designateMockClient) ForEachZone(handler func(zone *zones.Zone) error) error {
	for _, tz := range c.tzmap {
		if err := handler(tz.zone); err != nil {
			return err
		}
	}
	return nil
}

func (c *designateMockClient) ForEachRecordSet(zoneID string, handler func(recordSet *recordsets.RecordSet) error) error {
	return c.ForEachRecordSetFilterByTypeAndName(zoneID, "", "", handler)
}

func (c *designateMockClient) ForEachRecordSetFilterByTypeAndName(zoneID string, rrtype string, name string, handler func(recordSet *recordsets.RecordSet) error) error {
	tz := c.tzmap[zoneID]
	if tz == nil {
		return nil
	}
	for domainName, rssub := range tz.rsmap {
		if name != "" && domainName != name {
			continue
		}
		for _, rs := range rssub {
			if rrtype != "" && rs.Type != rrtype {
				continue
			}
			if err := handler(rs); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *designateMockClient) CreateRecordSet(zoneID string, opts recordsets.CreateOpts) (string, error) {
	tz := c.tzmap[zoneID]
	if tz == nil {
		return "", fmt.Errorf("Zone %s not found", zoneID)
	}
	if !strings.HasSuffix(opts.Name, tz.zone.Name) {
		return "", fmt.Errorf("Zone %s (%s): Invalid domain name: %s", zoneID, tz.zone.Name, opts.Name)
	}
	rssub, ok := tz.rsmap[opts.Name]
	if !ok {
		rssub = map[string]*recordsets.RecordSet{}
		tz.rsmap[opts.Name] = rssub
	}
	if _, ok := rssub[opts.Type]; ok {
		return "", fmt.Errorf("Domain %s: duplicate recordsets %s", opts.Name, opts.Type)
	}
	rs := recordsets.RecordSet{
		ID:      tz.buildNextId(),
		Name:    opts.Name,
		Type:    opts.Type,
		TTL:     opts.TTL,
		Records: opts.Records,
	}
	rssub[opts.Type] = &rs
	tz.id2rs[rs.ID] = &rs
	return rs.ID, nil
}

func (c *designateMockClient) getRecordSet(zoneID, recordSetID string) (*testzone, *recordsets.RecordSet, error) {
	tz := c.tzmap[zoneID]
	if tz == nil {
		return nil, nil, fmt.Errorf("Zone %s not found", zoneID)
	}
	rs, ok := tz.id2rs[recordSetID]
	if !ok {
		return nil, nil, fmt.Errorf("RecordSet not found: zone=%s, id=%s", zoneID, recordSetID)
	}
	return tz, rs, nil
}

func (c *designateMockClient) UpdateRecordSet(zoneID, recordSetID string, opts recordsets.UpdateOpts) error {
	_, rs, err := c.getRecordSet(zoneID, recordSetID)
	if err != nil {
		return err
	}
	rs.TTL = *opts.TTL
	rs.Records = opts.Records
	return nil
}

func (c *designateMockClient) DeleteRecordSet(zoneID, recordSetID string) error {
	tz, rs, err := c.getRecordSet(zoneID, recordSetID)
	if err != nil {
		return err
	}
	delete(tz.id2rs, recordSetID)
	delete(tz.rsmap[rs.Name], rs.Type)
	return nil
}

func newMockHandler(mockZones ...*zones.Zone) *Handler {
	c := designateMockClient{
		tzmap: map[string]*testzone{},
	}

	for _, z := range mockZones {
		c.tzmap[z.ID] = &testzone{
			zone:  z,
			rsmap: map[string]map[string]*recordsets.RecordSet{},
			id2rs: map[string]*recordsets.RecordSet{},
		}
	}

	var rateLimiterConfig *provider.RateLimiterConfig
	rateLimiter, _ := rateLimiterConfig.NewRateLimiter()

	h := &Handler{
		client: &c,
		config: provider.DNSHandlerConfig{
			RateLimiter: rateLimiter,
		},
	}

	cacheFactory := provider.NewTestZoneCacheFactory(60*time.Second, 0*time.Second)
	cache, _ := cacheFactory.CreateZoneCache(provider.CacheZoneState, mockMetrics, h.getZones, h.getZoneState)
	h.cache = cache
	h.config.Options = &provider.FactoryOptions{
		GenericFactoryOptions: provider.GenericFactoryOptions{},
	}
	return h
}

func newPreparedMockHandler(t *testing.T) *Handler {
	h := newMockHandler(
		&zones.Zone{
			ID:   "z1",
			Name: "z1.test.",
		},
		&zones.Zone{
			ID:   "z2",
			Name: "z2.test.",
		})
	_, err := h.client.CreateRecordSet("z2", recordsets.CreateOpts{
		Name:    "excluded.z2.test.",
		Type:    "NS",
		TTL:     3600,
		Records: []string{"ns1.somewhere", "ns2.somewhere"},
	})
	if err != nil {
		t.Errorf("Error on creating mock recordsets: %v", err)
	}
	return h
}

func TestGetZones(t *testing.T) {
	h := newPreparedMockHandler(t)
	hostedZones, err := h.GetZones()
	if err != nil {
		t.Error(err)
	}
	if len(hostedZones) != 2 {
		t.Errorf("Excepted 2 zones, but got %d", len(hostedZones))
	}
	sort.Slice(hostedZones, func(i, j int) bool {
		return strings.Compare(hostedZones[i].Id().ID, hostedZones[j].Id().ID) < 0
	})
	z1 := hostedZones[0]
	z2 := hostedZones[1]
	if z1.Id().ID != "z1" || z1.Domain() != "z1.test" {
		t.Errorf("Zone z1 not found: %v", z1)
	}
	if len(z1.ForwardedDomains()) != 0 {
		t.Errorf("Zone z1: unexpected forwarded domains: %v", z1.ForwardedDomains())
	}
	if z2.Id().ID != "z2" || z2.Domain() != "z2.test" {
		t.Errorf("Zone z2 not found: %v", z2)
	}
	if len(z2.ForwardedDomains()) != 1 || z2.ForwardedDomains()[0] != "excluded.z2.test" {
		t.Errorf("Zone z2: unexpected forwarded domains: %v", z2.ForwardedDomains())
	}
}

func getDNSHostedZone(h *Handler, zoneID string) (provider.DNSHostedZone, error) {
	tz, ok := h.client.(*designateMockClient).tzmap[zoneID]
	if !ok {
		return nil, fmt.Errorf("Zone %s not found", zoneID)
	}
	return tz, nil
}

func buildRecordSet(rrtype string, ttl int, recordValues ...string) *dns.RecordSet {
	records := dns.Records{}
	for _, value := range recordValues {
		records = append(records, &dns.Record{Value: value})
	}
	return &dns.RecordSet{Type: rrtype, TTL: int64(ttl), Records: records}
}

func TestGetZoneStateAndExecuteRequests(t *testing.T) {
	RegisterTestingT(t)
	h := newPreparedMockHandler(t)

	hostedZone, err := getDNSHostedZone(h, "z1")
	Ω(err).Should(BeNil(), "Get Zone z1 failed")

	zoneState, err := h.GetZoneState(hostedZone)
	Ω(err).Should(BeNil(), "Initial GetZoneState failed")
	dnssets := zoneState.GetDNSSets()
	Ω(len(dnssets)).Should(Equal(0), "dnssets should be empty initially")

	initial := []recordsets.CreateOpts{
		{
			Name:    "sub1.z1.test.",
			TTL:     301,
			Type:    "A",
			Records: []string{"1.2.3.4", "5.6.7.8"},
		},
		{
			Name:    "comment-sub1.z1.test.",
			TTL:     600,
			Type:    "TXT",
			Records: []string{"\"owner=test\"", "\"prefix=comment-\""},
		},
		{
			Name:    "sub2.z1.test.",
			TTL:     302,
			Type:    "CNAME",
			Records: []string{"cname.target.test."},
		},
		{
			Name:    "comment-sub2.z1.test.",
			TTL:     600,
			Type:    "TXT",
			Records: []string{"\"owner=test\"", "\"prefix=comment-\""},
		},
		{
			Name:    "sub3.z1.test.",
			TTL:     303,
			Type:    "TXT",
			Records: []string{"foo", "bar"},
		},
	}
	for _, opts := range initial {
		_, err = h.client.CreateRecordSet("z1", opts)
		Ω(err).Should(BeNil(), fmt.Sprintf("CreateRecordSet failed for %s %s", opts.Name, opts.Type))
	}

	stdMeta := buildRecordSet("META", 600, "\"owner=test\"", "\"prefix=comment-\"")
	sub1 := dns.DNSSetName{DNSName: "sub1.z1.test"}
	sub2 := dns.DNSSetName{DNSName: "sub2.z1.test"}
	sub3 := dns.DNSSetName{DNSName: "sub3.z1.test"}
	expectedDnssets := dns.DNSSets{
		sub1: &dns.DNSSet{
			Name: sub1,
			Sets: dns.RecordSets{
				"A":    buildRecordSet("A", 301, "1.2.3.4", "5.6.7.8"),
				"META": stdMeta,
			},
		},
		sub2: &dns.DNSSet{
			Name: sub2,
			Sets: dns.RecordSets{
				"CNAME": buildRecordSet("CNAME", 302, "cname.target.test"),
				"META":  stdMeta,
			},
		},
		dns.DNSSetName{DNSName: "sub3.z1.test"}: &dns.DNSSet{
			Name: sub3,
			Sets: dns.RecordSets{
				"TXT": buildRecordSet("TXT", 303, "foo", "bar"),
			},
		},
	}

	zoneState2, err := h.GetZoneState(hostedZone)
	Ω(err).Should(BeNil(), "GetZoneState failed")
	actualDnssets := zoneState2.GetDNSSets()
	Ω(actualDnssets).Should(Equal(expectedDnssets))

	tlog := logger.New()
	sub4 := dns.DNSSetName{DNSName: "sub4.z1.test"}
	reqs := []*provider.ChangeRequest{
		{
			Action: provider.R_CREATE,
			Type:   "A",
			Addition: &dns.DNSSet{
				Name: sub4,
				Sets: dns.RecordSets{
					"A": buildRecordSet("A", 304, "11.22.33.44"),
				},
			},
		},
		{
			Action: provider.R_CREATE,
			Type:   "META",
			Addition: &dns.DNSSet{
				Name: sub4,
				Sets: dns.RecordSets{
					"META": stdMeta,
				},
			},
		},
		{
			Action: provider.R_UPDATE,
			Type:   "A",
			Addition: &dns.DNSSet{
				Name: sub1,
				Sets: dns.RecordSets{
					"A": buildRecordSet("A", 305, "1.2.3.55", "5.6.7.8"),
				},
			},
		},
		{
			Action:   provider.R_DELETE,
			Type:     "CNAME",
			Deletion: expectedDnssets[sub2],
		},
		{
			Action:   provider.R_DELETE,
			Type:     "META",
			Deletion: expectedDnssets[sub2],
		},
		{
			Action:   provider.R_DELETE,
			Type:     "TXT",
			Deletion: expectedDnssets[sub3],
		},
	}
	err = h.ExecuteRequests(tlog, hostedZone, zoneState2, reqs)
	Ω(err).Should(BeNil(), "ExecuteRequests failed")

	expectedDnssets2 := dns.DNSSets{
		sub1: &dns.DNSSet{
			Name: sub1,
			Sets: dns.RecordSets{
				"A":    buildRecordSet("A", 305, "1.2.3.55", "5.6.7.8"),
				"META": stdMeta,
			},
		},
		sub4: &dns.DNSSet{
			Name: sub4,
			Sets: dns.RecordSets{
				"A":    buildRecordSet("A", 304, "11.22.33.44"),
				"META": stdMeta,
			},
		},
	}

	zoneState3, err := h.GetZoneState(hostedZone)
	if err != nil {
		t.Errorf("Second GetZoneState for z1 failed with: %v", err)
		return
	}
	actualDnssets2 := zoneState3.GetDNSSets()
	Ω(actualDnssets2[sub1]).Should(Equal(expectedDnssets2[sub1]))
	Ω(actualDnssets2[sub4]).Should(Equal(expectedDnssets2[sub4]))
	Ω(actualDnssets2).Should(Equal(expectedDnssets2))
}

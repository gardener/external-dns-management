// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package raw_test

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/raw"
)

type mockExecutor struct {
	records           raw.RecordList
	routingPolicies   []*dns.RoutingPolicy
	createdRecords    []raw.Record
	updatedRecords    []raw.Record
	deletedRecords    []raw.Record
	shouldFail        bool
	shouldFailGetList bool
}

func (m *mockExecutor) CreateRecord(_ context.Context, r raw.Record, _ provider.DNSHostedZone) error {
	if m.shouldFail {
		return fmt.Errorf("create failed")
	}
	m.createdRecords = append(m.createdRecords, r)
	return nil
}

func (m *mockExecutor) UpdateRecord(_ context.Context, r raw.Record, _ provider.DNSHostedZone) error {
	if m.shouldFail {
		return fmt.Errorf("update failed")
	}
	m.updatedRecords = append(m.updatedRecords, r)
	return nil
}

func (m *mockExecutor) DeleteRecord(_ context.Context, r raw.Record, _ provider.DNSHostedZone) error {
	if m.shouldFail {
		return fmt.Errorf("delete failed")
	}
	m.deletedRecords = append(m.deletedRecords, r)
	return nil
}

func (m *mockExecutor) NewRecord(fqdn, rtype, value string, _ provider.DNSHostedZone, ttl int64) raw.Record {
	return &mockRecord{
		dnsName: fqdn,
		rtype:   rtype,
		value:   value,
		ttl:     ttl,
	}
}

func (m *mockExecutor) GetRecordList(_ context.Context, _, _ string, _ provider.DNSHostedZone) (raw.RecordList, []*dns.RoutingPolicy, error) {
	if m.shouldFailGetList {
		return nil, nil, fmt.Errorf("get record list failed")
	}
	return m.records, m.routingPolicies, nil
}

type mockRecord struct {
	id            string
	dnsName       string
	rtype         string
	value         string
	ttl           int64
	setIdentifier string
	rp            *dns.RoutingPolicy
}

func (r *mockRecord) GetId() string                        { return r.id }
func (r *mockRecord) GetDNSName() string                   { return r.dnsName }
func (r *mockRecord) GetType() string                      { return r.rtype }
func (r *mockRecord) GetValue() string                     { return r.value }
func (r *mockRecord) GetTTL() int64                        { return r.ttl }
func (r *mockRecord) GetSetIdentifier() string             { return r.setIdentifier }
func (r *mockRecord) GetRoutingPolicy() *dns.RoutingPolicy { return r.rp }
func (r *mockRecord) SetTTL(ttl int64)                     { r.ttl = ttl }
func (r *mockRecord) SetRoutingPolicy(id string, rp *dns.RoutingPolicy) {
	r.setIdentifier = id
	r.rp = rp
}
func (r *mockRecord) Clone() raw.Record {
	clone := *r
	return &clone
}

var _ = Describe("Execution", func() {
	var (
		ctx      context.Context
		executor *mockExecutor
		zone     provider.DNSHostedZone
		log      logr.Logger
		rs1      = &dns.RecordSet{
			Type: dns.TypeA,
			TTL:  300,
			Records: []*dns.Record{
				{Value: "1.2.3.4"},
			},
		}
		mr1 = &mockRecord{
			id:      "mr1",
			dnsName: "test.example.com",
			rtype:   string(dns.TypeA),
			value:   "1.2.3.4",
			ttl:     300,
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		executor = &mockExecutor{}
		zone = dns.NewZoneInfo(dns.ZoneID{ProviderType: "mock", ID: "test-zone"}, "example.com", false, "key1")
		log = logr.Discard()
	})

	It("should handle creation of new records", func() {
		name := dns.DNSSetName{DNSName: "test.example.com"}
		reqs := provider.ChangeRequests{
			Name: name,
			Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
				dns.TypeA: {
					New: rs1,
				},
			},
		}

		err := raw.ExecuteRequests(ctx, log, executor, zone, reqs, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(executor.createdRecords).To(HaveLen(1))
		Expect(executor.updatedRecords).To(BeEmpty())
		Expect(executor.deletedRecords).To(BeEmpty())

		Expect(executor.createdRecords[0].GetValue()).To(Equal("1.2.3.4"))
		Expect(executor.createdRecords[0].GetTTL()).To(Equal(int64(300)))

		executor.shouldFailGetList = true
		err = raw.ExecuteRequests(ctx, log, executor, zone, reqs, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("get record list failed"))
	})

	It("should handle failures gracefully", func() {
		executor.shouldFail = true
		name := dns.DNSSetName{DNSName: "test.example.com"}
		reqs := provider.ChangeRequests{
			Name: name,
			Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
				dns.TypeA: {
					New: rs1,
				},
			},
		}

		err := raw.ExecuteRequests(ctx, log, executor, zone, reqs, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("could not update all dns entries"))
	})

	It("should handle unsupported routing policy gracefully", func() {
		name := dns.DNSSetName{DNSName: "test.example.com", SetIdentifier: "test"}
		reqs := provider.ChangeRequests{
			Name: name,
			Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
				dns.TypeA: {
					New: rs1,
				},
			},
		}

		err := raw.ExecuteRequests(ctx, log, executor, zone, reqs, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("routing policy not supported"))
	})

	It("should handle updates of existing records", func() {
		executor.records = raw.RecordList{
			mr1,
		}

		name := dns.DNSSetName{DNSName: "test.example.com"}
		rs2 := rs1.Clone()
		rs2.TTL = 600
		reqs := provider.ChangeRequests{
			Name: name,
			Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
				dns.TypeA: {
					Old: rs1,
					New: rs2,
				},
			},
		}

		err := raw.ExecuteRequests(ctx, log, executor, zone, reqs, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(executor.createdRecords).To(BeEmpty())
		Expect(executor.updatedRecords).To(HaveLen(1))
		Expect(executor.deletedRecords).To(BeEmpty())
		Expect(executor.updatedRecords[0].GetTTL()).To(Equal(int64(600)))

		executor.shouldFailGetList = true
		err = raw.ExecuteRequests(ctx, log, executor, zone, reqs, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("get record list failed"))
	})

	It("should handle deletion of existing records", func() {
		executor.records = raw.RecordList{
			mr1,
		}

		name := dns.DNSSetName{DNSName: "test.example.com"}
		reqs := provider.ChangeRequests{
			Name: name,
			Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
				dns.TypeA: {
					Old: rs1,
				},
			},
		}

		err := raw.ExecuteRequests(ctx, log, executor, zone, reqs, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(executor.createdRecords).To(BeEmpty())
		Expect(executor.updatedRecords).To(BeEmpty())
		Expect(executor.deletedRecords).To(HaveLen(1))

		executor.shouldFailGetList = true
		err = raw.ExecuteRequests(ctx, log, executor, zone, reqs, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("get record list failed"))
	})

	It("should handle request without changes", func() {
		executor.records = raw.RecordList{
			mr1,
		}

		name := dns.DNSSetName{DNSName: "test.example.com"}
		reqs := provider.ChangeRequests{
			Name: name,
			Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
				dns.TypeA: {
					Old: rs1,
					New: rs1,
				},
			},
		}

		err := raw.ExecuteRequests(ctx, log, executor, zone, reqs, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(executor.createdRecords).To(BeEmpty())
		Expect(executor.updatedRecords).To(BeEmpty())
		Expect(executor.deletedRecords).To(BeEmpty())
	})

	It("should handle obsolete existing records", func() {
		executor.records = raw.RecordList{
			mr1,
			&mockRecord{
				id:      "mr2",
				dnsName: "test.example.com",
				rtype:   string(dns.TypeA),
				value:   "5.6.7.8",
				ttl:     300,
			},
		}

		name := dns.DNSSetName{DNSName: "test.example.com"}
		rs2 := rs1.Clone()
		rs2.Type = dns.TypeCNAME
		rs2.Records[0].Value = "test.forwarded.example.com"
		reqs := provider.ChangeRequests{
			Name: name,
			Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
				dns.TypeA: {
					Old: rs1,
				},
				dns.TypeCNAME: {
					New: rs2,
				},
			},
		}

		err := raw.ExecuteRequests(ctx, log, executor, zone, reqs, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(executor.createdRecords).To(HaveLen(1))
		Expect(executor.updatedRecords).To(BeEmpty())
		Expect(executor.deletedRecords).To(HaveLen(2))
	})
})

// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package openstack

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

// noopMetrics is a provider.Metrics implementation that records nothing.
type noopMetrics struct{}

func (noopMetrics) AddGenericRequests(_ provider.MetricsRequestType, _ int)        {}
func (noopMetrics) AddZoneRequests(_ string, _ provider.MetricsRequestType, _ int) {}

// fakeDesignateClient is a controllable fake of designateClientInterface used by the OpenStack handler tests.
// existing holds the record sets the fake pretends the zone contains (matched by name, mirroring the real
// lookup). lookupErr, if set, is returned by ForEachRecordSetFilterByTypeAndName to simulate an API failure.
type fakeDesignateClient struct {
	existing  []*recordsets.RecordSet
	lookupErr error

	createCalls []recordsets.CreateOpts
	updateCalls []string // record set IDs passed to UpdateRecordSet
	deleteCalls []string // record set IDs passed to DeleteRecordSet
}

func (f *fakeDesignateClient) ForEachZone(_ context.Context, _ func(zone *zones.Zone) error) error {
	return errors.New("not implemented")
}

func (f *fakeDesignateClient) ForEachRecordSet(_ context.Context, _ string, _ func(recordSet *recordsets.RecordSet) error) error {
	return errors.New("not implemented")
}

func (f *fakeDesignateClient) ForEachRecordSetFilterByTypeAndName(_ context.Context, _ string, _ string, name string, handler func(recordSet *recordsets.RecordSet) error) error {
	if f.lookupErr != nil {
		return f.lookupErr
	}
	for _, rs := range f.existing {
		if rs.Name == name {
			if err := handler(rs); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *fakeDesignateClient) CreateRecordSet(_ context.Context, _ string, opts recordsets.CreateOpts) (string, error) {
	f.createCalls = append(f.createCalls, opts)
	return "new-id", nil
}

func (f *fakeDesignateClient) UpdateRecordSet(_ context.Context, _, recordSetID string, _ recordsets.UpdateOpts) error {
	f.updateCalls = append(f.updateCalls, recordSetID)
	return nil
}

func (f *fakeDesignateClient) DeleteRecordSet(_ context.Context, _, recordSetID string) error {
	f.deleteCalls = append(f.deleteCalls, recordSetID)
	return nil
}

var _ designateClientInterface = &fakeDesignateClient{}

var _ = Describe("execution", func() {
	var (
		ctx    context.Context
		client *fakeDesignateClient
		exec   *execution
		rs     *recordsets.RecordSet
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = &fakeDesignateClient{}
		h := &handler{
			config: provider.DNSHandlerConfig{
				Metrics:     noopMetrics{},
				RateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
			},
			client: client,
		}
		zone := provider.NewDNSHostedZone(ProviderType, "zone-id", "example.com", "", false)
		exec = newExecution(logr.Discard(), h, zone)
		rs = &recordsets.RecordSet{
			Name:    "foo.example.com.",
			Type:    string(dns.TypeA),
			TTL:     300,
			Records: []string{"1.2.3.4"},
		}
	})

	Describe("update", func() {
		It("falls back to create when the record set does not exist (wildcard-synthesized actual state)", func() {
			Expect(exec.update(ctx, rs)).To(Succeed())

			Expect(client.createCalls).To(HaveLen(1))
			Expect(client.createCalls[0].Name).To(Equal("foo.example.com."))
			Expect(client.updateCalls).To(BeEmpty())
		})

		It("updates the existing record set when it is found", func() {
			client.existing = []*recordsets.RecordSet{{ID: "existing-id", Name: "foo.example.com.", Type: string(dns.TypeA)}}

			Expect(exec.update(ctx, rs)).To(Succeed())

			Expect(client.updateCalls).To(Equal([]string{"existing-id"}))
			Expect(client.createCalls).To(BeEmpty())
		})

		It("propagates a genuine lookup error without falling back to create", func() {
			client.lookupErr = errors.New("designate unavailable")

			err := exec.update(ctx, rs)
			Expect(err).To(HaveOccurred())
			Expect(client.createCalls).To(BeEmpty())
			Expect(client.updateCalls).To(BeEmpty())
		})
	})

	Describe("delete", func() {
		It("is a no-op success when the record set does not exist", func() {
			Expect(exec.delete(ctx, rs)).To(Succeed())

			Expect(client.deleteCalls).To(BeEmpty())
		})

		It("deletes the existing record set when it is found", func() {
			client.existing = []*recordsets.RecordSet{{ID: "existing-id", Name: "foo.example.com.", Type: string(dns.TypeA)}}

			Expect(exec.delete(ctx, rs)).To(Succeed())

			Expect(client.deleteCalls).To(Equal([]string{"existing-id"}))
		})

		It("propagates a genuine lookup error", func() {
			client.lookupErr = errors.New("designate unavailable")

			Expect(exec.delete(ctx, rs)).To(HaveOccurred())
			Expect(client.deleteCalls).To(BeEmpty())
		})
	})
})

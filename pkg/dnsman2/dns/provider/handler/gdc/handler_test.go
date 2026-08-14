// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gdc

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	globalnetworkingv1 "gke-internal.googlesource.com/private-cloud/pkg/apis/public/global/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	dnsconst "github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/dns"
)

type noopMetrics struct{}

func (m *noopMetrics) AddGenericRequests(_ provider.MetricsRequestType, _ int) {}
func (m *noopMetrics) AddZoneRequests(_ string, _ provider.MetricsRequestType, _ int) {
}

func TestGetZones(t *testing.T) {
	type zoneResult struct {
		ID     string
		Domain string
	}

	tests := []struct {
		name          string
		existingZones []runtime.Object
		expected      []zoneResult
		expectErr     bool
	}{
		{
			name: "discover existing zones",
			existingZones: []runtime.Object{
				&globalnetworkingv1.ManagedDNSZone{
					ObjectMeta: metav1.ObjectMeta{Name: "my-zone-1", Namespace: "my-project"},
					Spec:       globalnetworkingv1.ManagedDNSZoneSpec{DNSName: "zone1.gdc.sap.corp."},
				},
				&globalnetworkingv1.ManagedDNSZone{
					ObjectMeta: metav1.ObjectMeta{Name: "my-zone-2", Namespace: "my-project"},
					Spec:       globalnetworkingv1.ManagedDNSZoneSpec{DNSName: "zone2.gdc.sap.corp."},
				},
			},
			expected: []zoneResult{
				{ID: "my-project/my-zone-1", Domain: "zone1.gdc.sap.corp"},
				{ID: "my-project/my-zone-2", Domain: "zone2.gdc.sap.corp"},
			},
			expectErr: false,
		},
		{
			name:          "no zones found",
			existingZones: []runtime.Object{},
			expected:      nil,
			expectErr:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			scheme := runtime.NewScheme()
			_ = globalnetworkingv1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tc.existingZones...).Build()
			h := &handler{
				project: "my-project",
				client:  fakeClient,
				config: provider.DNSHandlerConfig{
					Log:         log.Log,
					Metrics:     &noopMetrics{},
					RateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
				},
			}
			ctx := context.Background()

			// Action
			zones, err := h.GetZones(ctx)

			// Assert
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected an error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error listing zones: %v", err)
			}

			var actual []zoneResult
			for _, z := range zones {
				actual = append(actual, zoneResult{
					ID:     z.ZoneID().ID,
					Domain: z.Domain(),
				})
			}

			if diff := cmp.Diff(tc.expected, actual); diff != "" {
				t.Errorf("GetZones mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestQueryDNS(t *testing.T) {
	ttlVal := uint32(300)
	tests := []struct {
		name            string
		existingRecords []runtime.Object
		queryName       string
		queryType       dns.RecordType
		expectedRS      *dns.RecordSet
	}{
		{
			name: "resolve existing record",
			existingRecords: []runtime.Object{
				&globalnetworkingv1.ResourceRecordSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dnsconst.GetDNSRecordSetName("A", "host.zone1.gdc.sap.corp"),
						Namespace: "my-project",
					},
					Spec: globalnetworkingv1.ResourceRecordSetSpec{
						Name:       "host.zone1.gdc.sap.corp.",
						Type:       "A",
						TTLSeconds: &ttlVal,
						RRData:     []string{"1.2.3.4", "5.6.7.8"},
					},
				},
			},
			queryName:  "host.zone1.gdc.sap.corp",
			queryType:  dns.TypeA,
			expectedRS: dns.NewRecordSet(dns.TypeA, 300, []*dns.Record{{Value: "1.2.3.4"}, {Value: "5.6.7.8"}}),
		},
		{
			name: "resolve TXT record with surrounding quotes",
			existingRecords: []runtime.Object{
				&globalnetworkingv1.ResourceRecordSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dnsconst.GetDNSRecordSetName("TXT", "txt.zone1.gdc.sap.corp"),
						Namespace: "my-project",
					},
					Spec: globalnetworkingv1.ResourceRecordSetSpec{
						Name:       "txt.zone1.gdc.sap.corp.",
						Type:       "TXT",
						TTLSeconds: &ttlVal,
						RRData:     []string{`"sample-text"`},
					},
				},
			},
			queryName:  "txt.zone1.gdc.sap.corp",
			queryType:  dns.TypeTXT,
			expectedRS: dns.NewRecordSet(dns.TypeTXT, 300, []*dns.Record{{Value: "sample-text"}}),
		},
		{
			name:            "record not found",
			existingRecords: []runtime.Object{},
			queryName:       "missing.zone1.gdc.sap.corp",
			queryType:       dns.TypeA,
			expectedRS:      nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			scheme := runtime.NewScheme()
			_ = globalnetworkingv1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tc.existingRecords...).Build()
			h := &handler{
				project: "my-project",
				client:  fakeClient,
				config: provider.DNSHandlerConfig{
					Log:         log.Log,
					Metrics:     &noopMetrics{},
					RateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
				},
			}
			ctx := context.Background()
			zoneInfo := dns.NewZoneInfo(dns.NewZoneID("gdch-dns", "my-project/my-zone-1"), "zone1.gdc.sap.corp.", false, "my-zone-1")
			setName := dns.DNSSetName{DNSName: tc.queryName}

			// Action
			rs, err := h.queryDNS(ctx, zoneInfo, setName, tc.queryType)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error querying dns: %v", err)
			}
			if diff := cmp.Diff(tc.expectedRS, rs, cmp.AllowUnexported(dns.RecordSet{})); diff != "" {
				t.Errorf("queryDNS mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestExecuteRequests(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = globalnetworkingv1.AddToScheme(scheme)

	tests := []struct {
		name            string
		existingRecords []runtime.Object
		requests        provider.ChangeRequests
		expectedSpec    *globalnetworkingv1.ResourceRecordSetSpec
		expectDeleted   bool
		expectErr       bool
	}{
		{
			name:            "create new record set",
			existingRecords: []runtime.Object{},
			requests: provider.ChangeRequests{
				Name: dns.DNSSetName{DNSName: "new-host.zone1.gdc.sap.corp"},
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {
						Old: nil,
						New: dns.NewRecordSet(dns.TypeA, 300, []*dns.Record{{Value: "10.0.0.1"}}),
					},
				},
			},
			expectedSpec: &globalnetworkingv1.ResourceRecordSetSpec{
				Name:       "new-host.zone1.gdc.sap.corp",
				Type:       "A",
				TTLSeconds: func() *uint32 { v := uint32(300); return &v }(),
				RRData:     []string{"10.0.0.1"},
				DNSZone:    "my-zone-1",
			},
			expectDeleted: false,
			expectErr:     false,
		},
		{
			name: "update existing record set",
			existingRecords: []runtime.Object{
				&globalnetworkingv1.ResourceRecordSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dnsconst.GetDNSRecordSetName("A", "host.zone1.gdc.sap.corp"),
						Namespace: "my-project",
					},
					Spec: globalnetworkingv1.ResourceRecordSetSpec{
						Name:       "host.zone1.gdc.sap.corp.",
						Type:       "A",
						TTLSeconds: func() *uint32 { v := uint32(300); return &v }(),
						RRData:     []string{"1.2.3.4"},
						DNSZone:    "my-zone-1",
					},
				},
			},
			requests: provider.ChangeRequests{
				Name: dns.DNSSetName{DNSName: "host.zone1.gdc.sap.corp"},
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {
						Old: dns.NewRecordSet(dns.TypeA, 300, []*dns.Record{{Value: "1.2.3.4"}}),
						New: dns.NewRecordSet(dns.TypeA, 600, []*dns.Record{{Value: "5.6.7.8"}, {Value: "9.10.11.12"}}),
					},
				},
			},
			expectedSpec: &globalnetworkingv1.ResourceRecordSetSpec{
				Name:       "host.zone1.gdc.sap.corp",
				Type:       "A",
				TTLSeconds: func() *uint32 { v := uint32(600); return &v }(),
				RRData:     []string{"5.6.7.8", "9.10.11.12"},
				DNSZone:    "my-zone-1",
			},
			expectDeleted: false,
			expectErr:     false,
		},
		{
			name: "delete existing record set",
			existingRecords: []runtime.Object{
				&globalnetworkingv1.ResourceRecordSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dnsconst.GetDNSRecordSetName("A", "host.zone1.gdc.sap.corp"),
						Namespace: "my-project",
					},
					Spec: globalnetworkingv1.ResourceRecordSetSpec{
						Name:       "host.zone1.gdc.sap.corp.",
						Type:       "A",
						TTLSeconds: func() *uint32 { v := uint32(300); return &v }(),
						RRData:     []string{"1.2.3.4"},
						DNSZone:    "my-zone-1",
					},
				},
			},
			requests: provider.ChangeRequests{
				Name: dns.DNSSetName{DNSName: "host.zone1.gdc.sap.corp"},
				Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
					dns.TypeA: {
						Old: dns.NewRecordSet(dns.TypeA, 300, []*dns.Record{{Value: "1.2.3.4"}}),
						New: nil,
					},
				},
			},
			expectedSpec:  nil,
			expectDeleted: true,
			expectErr:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tc.existingRecords...).Build()
			h := &handler{
				project: "my-project",
				client:  fakeClient,
				config: provider.DNSHandlerConfig{
					Log:         log.Log,
					Metrics:     &noopMetrics{},
					RateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
				},
			}
			ctx := context.Background()
			hostedZone := provider.NewDNSHostedZone("gdch-dns", "my-project/my-zone-1", "zone1.gdc.sap.corp.", "my-zone-1", false)

			// Action
			err := h.ExecuteRequests(ctx, hostedZone, tc.requests)

			// Assert
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected an error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			recordList := &globalnetworkingv1.ResourceRecordSetList{}
			if err := fakeClient.List(ctx, recordList, client.InNamespace("my-project")); err != nil {
				t.Fatalf("failed to list ResourceRecordSets: %v", err)
			}

			var actualSpecs []globalnetworkingv1.ResourceRecordSetSpec
			for _, item := range recordList.Items {
				actualSpecs = append(actualSpecs, item.Spec)
			}

			var expectedSpecs []globalnetworkingv1.ResourceRecordSetSpec
			if tc.expectedSpec != nil {
				expectedSpecs = append(expectedSpecs, *tc.expectedSpec)
			}

			if diff := cmp.Diff(expectedSpecs, actualSpecs); diff != "" {
				t.Errorf("ResourceRecordSets state mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetCustomQueryDNSFunc(t *testing.T) {
	// Arrange
	scheme := runtime.NewScheme()
	_ = globalnetworkingv1.AddToScheme(scheme)
	ttlVal := uint32(300)
	recordSet := &globalnetworkingv1.ResourceRecordSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dnsconst.GetDNSRecordSetName("A", "host.zone1.gdc.sap.corp"),
			Namespace: "my-project",
		},
		Spec: globalnetworkingv1.ResourceRecordSetSpec{
			Name:       "host.zone1.gdc.sap.corp.",
			Type:       "A",
			TTLSeconds: &ttlVal,
			RRData:     []string{"1.2.3.4"},
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(recordSet).Build()
	h := &handler{
		project: "my-project",
		client:  fakeClient,
		config: provider.DNSHandlerConfig{
			Log:         log.Log,
			Metrics:     &noopMetrics{},
			RateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
		},
	}
	zoneInfo := dns.NewZoneInfo(dns.NewZoneID("gdch-dns", "my-project/my-zone-1"), "zone1.gdc.sap.corp.", false, "my-zone-1")

	// Action
	queryFn, err := h.GetCustomQueryDNSFunc(zoneInfo, nil)

	// Assert
	if err != nil {
		t.Fatalf("unexpected error getting query function: %v", err)
	}

	ctx := context.Background()
	setName := dns.DNSSetName{DNSName: "host.zone1.gdc.sap.corp"}
	rs, err := queryFn(ctx, zoneInfo, setName, dns.TypeA)
	if err != nil {
		t.Fatalf("unexpected error running query function: %v", err)
	}

	expected := dns.NewRecordSet(dns.TypeA, 300, []*dns.Record{{Value: "1.2.3.4"}})
	if diff := cmp.Diff(expected, rs, cmp.AllowUnexported(dns.RecordSet{})); diff != "" {
		t.Errorf("custom resolver result mismatch (-want +got):\n%s", diff)
	}
}

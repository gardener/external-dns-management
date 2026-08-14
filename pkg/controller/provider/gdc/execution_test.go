// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gdc

import (
	"context"
	"testing"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/google/go-cmp/cmp"
	globalnetworkingv1 "gke-internal.googlesource.com/private-cloud/pkg/apis/public/global/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type MockMetrics struct{}

func (m *MockMetrics) AddGenericRequests(_ string, _ int) {}
func (m *MockMetrics) AddZoneRequests(_, _ string, _ int) {}

func TestApplyChange(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = globalnetworkingv1.AddToScheme(scheme)
	ttl := uint32(300)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	handler := &Handler{
		client:  fakeClient,
		project: "test-project",
		config: provider.DNSHandlerConfig{
			Metrics:     &MockMetrics{},
			RateLimiter: flowcontrol.NewTokenBucketRateLimiter(5.0, 10),
		},
	}

	exec := &Execution{
		LogContext: logger.NewContext("test", "applyChange"),
		handler:    handler,
		zone:       provider.NewDNSHostedZone("GDC", "test-zone-id", "test.com", "test-zone-key", true),
	}

	tests := []struct {
		name    string
		change  *Change
		wantRRS *globalnetworkingv1.ResourceRecordSet
	}{
		{
			name: "Create DNS record",
			change: &Change{
				RecordSet: &resourceRecordSet{
					fqdn: "test.example.com",
					ttl:  300,
					data: []string{"1.2.3.4"},
				},
				RecordType:     "A",
				ChangeType:     provider.R_CREATE,
				ObjectMetaName: "a-test.example.com",
			},
			wantRRS: &globalnetworkingv1.ResourceRecordSet{
				ObjectMeta: metav1.ObjectMeta{Name: "a-test.example.com", Namespace: "test-project", ResourceVersion: "1"},
				Spec: globalnetworkingv1.ResourceRecordSetSpec{
					Name:       "test.example.com",
					TTLSeconds: &ttl,
					Type:       "A",
					RRData:     []string{"1.2.3.4"},
					DNSZone:    "test-zone-key",
				},
			},
		},
		{
			name: "Update DNS record",
			change: &Change{
				RecordSet: &resourceRecordSet{
					fqdn: "test.example.com",
					ttl:  300,
					data: []string{"5.6.7.8"},
				},
				RecordType:     "A",
				ChangeType:     provider.R_UPDATE,
				ObjectMetaName: "a-test.example.com",
			},
			wantRRS: &globalnetworkingv1.ResourceRecordSet{
				ObjectMeta: metav1.ObjectMeta{Name: "a-test.example.com", Namespace: "test-project", ResourceVersion: "2"},
				Spec: globalnetworkingv1.ResourceRecordSetSpec{
					Name:       "test.example.com",
					TTLSeconds: &ttl,
					Type:       "A",
					RRData:     []string{"5.6.7.8"},
					DNSZone:    "test-zone-key",
				},
			},
		},
		{
			name: "Delete DNS record",
			change: &Change{
				RecordSet:      &resourceRecordSet{},
				RecordType:     "A",
				ChangeType:     provider.R_DELETE,
				ObjectMetaName: "a-test.example.com",
			},
			wantRRS: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := exec.applyChange(tt.change)
			if err != nil {
				t.Errorf("applyChange() Got error = %v, expected nil", err)
			}

			if tt.wantRRS != nil {
				got := &globalnetworkingv1.ResourceRecordSet{}
				err = fakeClient.Get(context.Background(), client.ObjectKey{Name: tt.wantRRS.Name, Namespace: tt.wantRRS.Namespace}, got)

				if err != nil {
					t.Errorf("failed to get created ResourceRecordSet: %v", err)
				}
				if diff := cmp.Diff(tt.wantRRS, got); diff != "" {
					t.Errorf("unexpected ResourceRecordSet (-want +got):\n%s", diff)
				}
			}
		})
	}
}

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gdc

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/coredns/corefile-migration/migration/corefile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/google/go-cmp/cmp"
	globalnetworkingv1 "github.com/googlecloudplatform/google-distributed-cloud-apis/pkg/apis/public/global/networking/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeutil "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/constants"
	dnsconst "github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/dns"
	"github.com/gardener/external-dns-management/pkg/dns"
	dnsProvider "github.com/gardener/external-dns-management/pkg/dns/provider"
)

const (
	project       = "fake-project"
	testZoneName  = "test-zone"
	testZoneName1 = "test-zone-1"
	testZoneName2 = "test-zone-2"
	testDomain    = "zone1.test"
	testFQDN1     = "test1.zone1.test"
	testFQDN2     = "test2.zone1.test"
	testTTL       = dnsconst.DNSDefaultTTL
)

var (
	dnsSetName1        = dns.DNSSetName{DNSName: testFQDN1}
	dnsSetNameAType1   = dns.DNSSetName{DNSName: fmt.Sprintf("a-%s", testFQDN1)}
	dnsSetNameTxtType1 = dns.DNSSetName{DNSName: fmt.Sprintf("txt-%s", testFQDN1)}
	dnsSetName2        = dns.DNSSetName{DNSName: testFQDN2}
	dnsSetNameAType2   = dns.DNSSetName{DNSName: fmt.Sprintf("a-%s", testFQDN2)}
	dnsSetNameTxtType2 = dns.DNSSetName{DNSName: fmt.Sprintf("txt-%s", testFQDN2)}
	testZoneDomain     = dnsProvider.NewDNSHostedZone(
		TYPE_CODE,
		fmt.Sprintf("%s/%s", project, testZoneName),
		testDomain,
		testZoneName,
		false,
	)
	testZone1 = dnsProvider.NewDNSHostedZone(
		TYPE_CODE,
		fmt.Sprintf("%s/%s", project, testZoneName1),
		testFQDN1,
		testZoneName1,
		false,
	)
	ttl = uint32(testTTL)
)

type Config = dnsProvider.DNSHandlerConfig

func TestNewHandler(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "should return error if empty configuration is provided",
			config:  Config{},
			wantErr: true,
		},
		{
			name: "should return error if config properties does not have service account json",
			config: Config{
				Properties: map[string]string{
					constants.GDCHConfigJSONField: "{}",
				},
			},
			wantErr: true,
		},
		{
			name: "should return error if config properties has invalid config",
			config: Config{
				Properties: map[string]string{
					constants.GDCHConfigJSONField:     "invalid-config",
					constants.ServiceAccountJSONField: "{}",
				},
			},
			wantErr: true,
		},
		{
			name: "should return error if config properties has invalid serviceaccount.json",
			config: Config{
				Properties: map[string]string{
					constants.GDCHConfigJSONField:     "{}",
					constants.ServiceAccountJSONField: "invalid-service-account",
				},
			},
			wantErr: true,
		},
		{
			name: "should return the handler with gdch-config",
			config: Config{
				Properties: map[string]string{
					constants.GDCHConfigJSONField:     "{}",
					constants.ServiceAccountJSONField: "{}",
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewHandler(&tc.config)

			if err == nil && tc.wantErr {
				t.Errorf("expected error calling NewHandler, got nil")
			}

			if err != nil && !tc.wantErr {
				t.Errorf("unexpected error calling NewHandler: %s", err)
			}
		})
	}
}

func TestGetZones(t *testing.T) {
	h := mockHandler(t, []client.Object{
		&globalnetworkingv1.ManagedDNSZone{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-zone",
				Namespace: project,
			},
			Spec: globalnetworkingv1.ManagedDNSZoneSpec{
				DNSName:    "test.zone",
				Visibility: "PRIVATE",
			},
		},
	})
	zones, err := h.GetZones()

	if err != nil {
		t.Errorf("GetZones() expects no error, got: %v", err)
	}

	var expectedZones dnsProvider.DNSHostedZones = []dnsProvider.DNSHostedZone{
		dnsProvider.NewDNSHostedZone(TYPE_CODE, "fake-project/test-zone", "test.zone", "test-zone", false),
	}
	// Cannot use cmp because it leads to panic of unexported field zoneid.
	if !zones.EquivalentTo(expectedZones) {
		t.Errorf("expected zones %s, got %s", expectedZones, zones)
	}
}

func TestGetZoneState(t *testing.T) {
	testcases := []struct {
		name              string
		rrSetCRName       string
		rrsetCR           *globalnetworkingv1.ResourceRecordSet
		wantDNSRecordSets dns.RecordSets
	}{
		{
			name:        "A record",
			rrSetCRName: fmt.Sprintf("a-%s", testFQDN1),
			rrsetCR: &globalnetworkingv1.ResourceRecordSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("a-%s", testFQDN1),
					Namespace: project,
				},
				Spec: globalnetworkingv1.ResourceRecordSetSpec{
					Name:       testFQDN1,
					TTLSeconds: &ttl,
					Type:       "A",
					RRData:     []string{"1.1.1.1"},
					DNSZone:    testZoneName1,
				},
			},
			wantDNSRecordSets: dns.RecordSets{
				"A": {
					Type:      "A",
					TTL:       dnsconst.DNSDefaultTTL,
					IgnoreTTL: false,
					Records: []*dns.Record{
						{
							Value: "1.1.1.1",
						},
					},
				},
			},
		},
		{
			name:        "TXT record",
			rrSetCRName: fmt.Sprintf("txt-%s", testFQDN1),
			rrsetCR: &globalnetworkingv1.ResourceRecordSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("txt-%s", testFQDN1),
					Namespace: project,
				},
				Spec: globalnetworkingv1.ResourceRecordSetSpec{
					Name:       testFQDN1,
					TTLSeconds: &ttl,
					Type:       "TXT",
					RRData: []string{
						`"foo_key=foo_val"`,
					},
					DNSZone: testZoneName1,
				},
			},
			wantDNSRecordSets: dns.RecordSets{
				"TXT": {
					Type:      "TXT",
					TTL:       dnsconst.DNSDefaultTTL,
					IgnoreTTL: false,
					Records: []*dns.Record{
						{
							Value: `"foo_key=foo_val"`,
						},
					},
				},
			},
		},
		{
			name:        "CNAME record",
			rrSetCRName: fmt.Sprintf("txt-%s", testFQDN1),
			rrsetCR: &globalnetworkingv1.ResourceRecordSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("txt-%s", testFQDN1),
					Namespace: project,
				},
				Spec: globalnetworkingv1.ResourceRecordSetSpec{
					Name:       testFQDN1,
					TTLSeconds: &ttl,
					Type:       "CNAME",
					RRData: []string{
						`"foo_key=foo_val"`,
					},
					DNSZone: testZoneName1,
				},
			},
			wantDNSRecordSets: dns.RecordSets{
				"CNAME": {
					Type:      "CNAME",
					TTL:       dnsconst.DNSDefaultTTL,
					IgnoreTTL: false,
					Records: []*dns.Record{
						{
							Value: `"foo_key=foo_val"`,
						},
					},
				},
			},
		},
		{
			name:        "PRT record",
			rrSetCRName: fmt.Sprintf("txt-%s", testFQDN1),
			rrsetCR: &globalnetworkingv1.ResourceRecordSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("txt-%s", testFQDN1),
					Namespace: project,
				},
				Spec: globalnetworkingv1.ResourceRecordSetSpec{
					Name:       testFQDN1,
					TTLSeconds: &ttl,
					Type:       "PRT",
					RRData: []string{
						`"foo_key=foo_val"`,
					},
					DNSZone: testZoneName1,
				},
			},
			wantDNSRecordSets: dns.RecordSets{
				"PRT": {
					Type:      "PRT",
					TTL:       dnsconst.DNSDefaultTTL,
					IgnoreTTL: false,
					Records: []*dns.Record{
						{
							Value: `"foo_key=foo_val"`,
						},
					},
				},
			},
		},
		{
			name:        "DS record",
			rrSetCRName: fmt.Sprintf("txt-%s", testFQDN1),
			rrsetCR: &globalnetworkingv1.ResourceRecordSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("txt-%s", testFQDN1),
					Namespace: project,
				},
				Spec: globalnetworkingv1.ResourceRecordSetSpec{
					Name:       testFQDN1,
					TTLSeconds: &ttl,
					Type:       "DS",
					RRData: []string{
						`"1 2 3 6BDE"`,
					},
					DNSZone: testZoneName1,
				},
			},
			wantDNSRecordSets: dns.RecordSets{
				"DS": {
					Type:      "DS",
					TTL:       dnsconst.DNSDefaultTTL,
					IgnoreTTL: false,
					Records: []*dns.Record{
						{
							Value: `"1 2 3 6BDE"`,
						},
					},
				},
			},
		},
		{
			name:        "MX record",
			rrSetCRName: fmt.Sprintf("txt-%s", testFQDN1),
			rrsetCR: &globalnetworkingv1.ResourceRecordSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("txt-%s", testFQDN1),
					Namespace: project,
				},
				Spec: globalnetworkingv1.ResourceRecordSetSpec{
					Name:       testFQDN1,
					TTLSeconds: &ttl,
					Type:       "MX",
					RRData: []string{
						`"10 mail.example.com."`,
					},
					DNSZone: testZoneName1,
				},
			},
			wantDNSRecordSets: dns.RecordSets{
				"MX": {
					Type:      "MX",
					TTL:       dnsconst.DNSDefaultTTL,
					IgnoreTTL: false,
					Records: []*dns.Record{
						{
							Value: `"10 mail.example.com."`,
						},
					},
				},
			},
		},
		{
			name:        "Not related records",
			rrSetCRName: fmt.Sprintf("ns-%s", testFQDN1),
			rrsetCR: &globalnetworkingv1.ResourceRecordSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("ns-%s", testFQDN1),
					Namespace: project,
				},
				Spec: globalnetworkingv1.ResourceRecordSetSpec{
					Name:       testFQDN1,
					TTLSeconds: &ttl,
					Type:       "NS",
					RRData: []string{
						"ns1.zone1.test",
						"ns2.zone1.test",
					},
					DNSZone: testZoneName1,
				},
			},
			wantDNSRecordSets: dns.RecordSets{},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			h := mockHandler(t, []client.Object{
				tc.rrsetCR,
			})

			got, err := h.GetZoneState(testZone1)
			if err != nil {
				t.Errorf("GetZoneState() expects no error, got: %v", err)
			}

			for _, v := range got.GetDNSSets() {
				expectedSets := &dns.DNSSet{
					Name: dns.DNSSetName{
						DNSName: tc.rrSetCRName,
					},
					Sets: tc.wantDNSRecordSets,
				}
				if diff := cmp.Diff(expectedSets, v); diff != "" {
					t.Errorf("DNSSet (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestExecuteRequests(t *testing.T) {
	// Initialize record sets for testFQDN1.
	initHandlerWithRecordSets := func() Handler {
		aRecord := &globalnetworkingv1.ResourceRecordSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("a-%s", testFQDN1),
				Namespace: project,
			},
			Spec: globalnetworkingv1.ResourceRecordSetSpec{
				Name:       testFQDN1,
				TTLSeconds: &ttl,
				Type:       "A",
				RRData:     []string{"1.1.1.1"},
				DNSZone:    testZoneName,
			},
		}

		txtRecord := &globalnetworkingv1.ResourceRecordSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("txt-%s", testFQDN1),
				Namespace: project,
			},
			Spec: globalnetworkingv1.ResourceRecordSetSpec{
				Name:       testFQDN1,
				TTLSeconds: &ttl,
				Type:       "TXT",
				RRData:     []string{`"foo_key=foo_val"`},
				DNSZone:    testZoneName,
			},
		}

		return *mockHandler(t, []client.Object{
			aRecord, txtRecord,
		})
	}

	testcases := []struct {
		name             string
		reqs             []*dnsProvider.ChangeRequest
		wantDNSSets      dns.DNSSets
		wantError        bool
		wantInvalidCount int
		wantFailedCount  int
	}{
		{
			name: "Create Requests for different dns set",
			reqs: []*dnsProvider.ChangeRequest{
				{
					Action: dnsProvider.R_CREATE,
					Type:   "A",
					Addition: &dns.DNSSet{
						Name: dnsSetName2,
						Sets: dns.RecordSets{
							"A": buildRecordSet("A", dnsconst.DNSDefaultTTL, "11.22.33.44"),
						},
						UpdateGroup: project,
					},
				},
				{
					Action: dnsProvider.R_CREATE,
					Type:   "TXT",
					Addition: &dns.DNSSet{
						Name: dnsSetName2,
						Sets: dns.RecordSets{
							"TXT": buildRecordSet("TXT", dnsconst.DNSDefaultTTL, `"bar_key=bar_val"`),
						},
						UpdateGroup: project,
					},
				},
			},
			wantDNSSets: dns.DNSSets{
				dnsSetNameAType1: &dns.DNSSet{
					Name: dnsSetNameAType1,
					Sets: dns.RecordSets{
						"A": buildRecordSet("A", dnsconst.DNSDefaultTTL, "1.1.1.1"),
					},
				},
				dnsSetNameTxtType1: &dns.DNSSet{
					Name: dnsSetNameTxtType1,
					Sets: dns.RecordSets{
						"TXT": buildRecordSet("TXT", dnsconst.DNSDefaultTTL, `"foo_key=foo_val"`),
					},
				},
				dnsSetNameAType2: &dns.DNSSet{
					Name: dnsSetNameAType2,
					Sets: dns.RecordSets{
						"A": buildRecordSet("A", dnsconst.DNSDefaultTTL, "11.22.33.44"),
					},
				},
				dnsSetNameTxtType2: &dns.DNSSet{
					Name: dnsSetNameTxtType2,
					Sets: dns.RecordSets{
						"TXT": buildRecordSet("TXT", dnsconst.DNSDefaultTTL, `"bar_key=bar_val"`),
					},
				},
			},
		},
		{
			name: "Update Requests for same dns set",
			reqs: []*dnsProvider.ChangeRequest{
				{
					Action: dnsProvider.R_UPDATE,
					Type:   "A",
					Addition: &dns.DNSSet{
						Name: dnsSetName1,
						Sets: dns.RecordSets{
							"A": buildRecordSet("A", dnsconst.DNSDefaultTTL, "11.22.33.44"),
						},
						UpdateGroup: project,
					},
				},
				{
					Action: dnsProvider.R_UPDATE,
					Type:   "TXT",
					Addition: &dns.DNSSet{
						Name: dnsSetName1,
						Sets: dns.RecordSets{
							"TXT": buildRecordSet("TXT", dnsconst.DNSDefaultTTL, `"bar_key=bar_val"`),
						},
						UpdateGroup: project,
					},
				},
			},
			wantDNSSets: dns.DNSSets{
				dnsSetNameAType1: &dns.DNSSet{
					Name: dnsSetNameAType1,
					Sets: dns.RecordSets{
						"A": buildRecordSet("A", dnsconst.DNSDefaultTTL, "11.22.33.44"),
					},
				},
				dnsSetNameTxtType1: &dns.DNSSet{
					Name: dnsSetNameTxtType1,
					Sets: dns.RecordSets{
						"TXT": buildRecordSet("TXT", dnsconst.DNSDefaultTTL, `"bar_key=bar_val"`),
					},
				},
			},
		},
		{
			name: "Delete Requests",
			reqs: []*dnsProvider.ChangeRequest{
				{
					Action: dnsProvider.R_DELETE,
					Type:   "A",
					Deletion: &dns.DNSSet{
						Name: dnsSetName1,
						Sets: dns.RecordSets{
							"A": buildRecordSet("A", dnsconst.DNSDefaultTTL, ""),
						},
						UpdateGroup: project,
					},
				},
				{
					Action: dnsProvider.R_DELETE,
					Type:   "TXT",
					Deletion: &dns.DNSSet{
						Name: dnsSetName1,
						Sets: dns.RecordSets{
							"TXT": buildRecordSet("TXT", dnsconst.DNSDefaultTTL, ""),
						},
						UpdateGroup: project,
					},
				},
			},
			wantDNSSets: dns.DNSSets{},
		},
		{
			name: "Requests with Invalid Resource Record Type",
			reqs: []*dnsProvider.ChangeRequest{
				{
					Action: dnsProvider.R_CREATE,
					Type:   "AAAA",
					Addition: &dns.DNSSet{
						Name: dnsSetName1,
						Sets: dns.RecordSets{
							"AAAA": buildRecordSet("AAAA", dnsconst.DNSDefaultTTL, "0000:0000:0000:0000:0000:0000:0000:0000"),
						},
					},
				},
				{
					Action: dnsProvider.R_DELETE,
					Type:   "NS",
					Deletion: &dns.DNSSet{
						Name: dnsSetName1,
						Sets: dns.RecordSets{
							"NS": buildRecordSet("NS", dnsconst.DNSDefaultTTL, "sub-vpc.zone1.test"),
						},
					},
				},
			},
			wantDNSSets: dns.DNSSets{
				dnsSetNameAType1: &dns.DNSSet{
					Name: dnsSetNameAType1,
					Sets: dns.RecordSets{
						"A": buildRecordSet("A", dnsconst.DNSDefaultTTL, "1.1.1.1"),
					},
				},
				dnsSetNameTxtType1: &dns.DNSSet{
					Name: dnsSetNameTxtType1,
					Sets: dns.RecordSets{
						"TXT": buildRecordSet("TXT", dnsconst.DNSDefaultTTL, `"foo_key=foo_val"`),
					},
				},
			},
			wantInvalidCount: 2,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			h := initHandlerWithRecordSets()
			zoneState, err := h.GetZoneState(testZoneDomain)
			if err != nil {
				t.Errorf("GetZoneState(%v) unexpect error: %v", testZoneDomain, err)
			}

			doneHandler := &testDoneHandler{}
			for _, r := range tc.reqs {
				r.Done = doneHandler
			}

			err = h.ExecuteRequests(logger.New(), testZoneDomain, zoneState, tc.reqs)
			if err != nil && !tc.wantError {
				t.Errorf("ExecuteRequests unexpect error: %v", err)
			}
			if err == nil && tc.wantError {
				t.Error("ExecuteRequests expects error but got nil")
			}

			if diff := cmp.Diff(doneHandler.invalidCount, tc.wantInvalidCount); diff != "" {
				t.Errorf("Invalid Count (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(doneHandler.failedCount, tc.wantFailedCount); diff != "" {
				t.Errorf("Failed Count (-want +got):\n%s", diff)
			}

			got, err := h.GetZoneState(testZoneDomain)
			if err != nil {
				t.Errorf("GetZoneState(%v) unexpect error : %v", testZoneDomain, err)
			}

			if diff := cmp.Diff(sortDNSSets(tc.wantDNSSets), sortDNSSets(got.GetDNSSets())); diff != "" {
				t.Errorf("DNSSet (-want +got):\n%s", diff)
			}
		})
	}
}

func TestZoneNames(t *testing.T) {
	tests := []struct {
		name     string
		corefile *corefile.Corefile
		want     []string
	}{
		{
			name: "simple corefile",
			corefile: &corefile.Corefile{
				Servers: []*corefile.Server{
					{
						DomPorts: []string{"example-1.com"},
						Plugins:  nil,
					},
				},
			},
			want: []string{"example-1.com"},
		},
		{
			name: "complex corefile",
			corefile: &corefile.Corefile{
				Servers: []*corefile.Server{
					{
						DomPorts: []string{"example-1.com", "example-2.com"},
						Plugins:  nil,
					},
					{
						DomPorts: []string{"example-1.com:123", "example-2.com:123"},
						Plugins:  nil,
					},
				},
			},
			want: []string{"example-1.com", "example-2.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := zoneNames(tc.corefile)

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("zoneNames(%v) (-want +got):\n%s", tc.corefile, diff)
			}
		})
	}
}

func TestMakeAndSplitZoneID(t *testing.T) {
	h := mockHandler(t, []client.Object{})
	projectName := "fake-project"
	h.project = projectName
	zoneName := "fake-zone-name"
	zoneID := h.makeZoneID(zoneName)

	if zoneID != "fake-project/fake-zone-name" {
		t.Errorf("makeZoneID(%v) = %v, want %v", zoneName, zoneID, "fake-project/fake-zone-name")
	}

	gotProjectName, gotZoneName, err := SplitZoneID(zoneID)
	if err != nil {
		t.Errorf("SplitZoneID(%v) = (%v, %v), err %v", zoneID, gotZoneName, gotProjectName, err)
	}
	if gotZoneName != zoneName || gotProjectName != projectName {
		t.Errorf("SplitZoneID(%v) = (%v, %v), want (%v, %v)", zoneID, gotZoneName, gotProjectName, zoneName, projectName)
	}
}

func mockHandler(t *testing.T, objects []client.Object) *Handler {
	c := initTestClient(objects)

	var rateLimiterConfig *dnsProvider.RateLimiterConfig
	rateLimiter, _ := rateLimiterConfig.NewRateLimiter()

	h := &Handler{
		client:            c,
		DefaultDNSHandler: dnsProvider.NewDefaultDNSHandler(TYPE_CODE),
		config: Config{
			Metrics: &dnsProvider.NullMetrics{},
			Options: &dnsProvider.FactoryOptions{
				GenericFactoryOptions: dnsProvider.GenericFactoryOptions{},
			},
			RateLimiter: rateLimiter,
		},
		project: project,
	}

	cacheFactory := dnsProvider.NewTestZoneCacheFactory(60*time.Second, 0*time.Second)
	cache, err := cacheFactory.CreateZoneCache(dnsProvider.CacheZoneState, h.config.Metrics, h.getZones, h.getZoneState)
	if err != nil {
		t.Fatal("fail to initialize zone cache of handler")
	}
	h.cache = cache

	return h
}

func initTestClient(objects []client.Object) client.WithWatch {
	scheme := runtime.NewScheme()
	runtimeutil.Must(globalnetworkingv1.AddToScheme(scheme))
	runtimeutil.Must(corev1.AddToScheme(scheme))
	src := fake.NewClientBuilder().WithScheme(scheme)
	if len(objects) > 0 {
		src = src.WithObjects(objects...)
	}
	return src.Build()
}

func buildRecordSet(rrtype string, ttl int, recordValues ...string) *dns.RecordSet {
	var records dns.Records
	for _, value := range recordValues {
		records = append(records, &dns.Record{Value: value})
	}
	return &dns.RecordSet{Type: rrtype, TTL: int64(ttl), Records: records}
}

type testDoneHandler struct {
	invalidCount int
	failedCount  int
	lastMessage  string
}

var _ dnsProvider.DoneHandler = &testDoneHandler{}

func (h *testDoneHandler) SetInvalid(err error) {
	h.invalidCount++
	h.lastMessage = err.Error()
}

func (h *testDoneHandler) Failed(err error) {
	h.failedCount++
	h.lastMessage = err.Error()
}

func (h *testDoneHandler) Throttled() {}
func (h *testDoneHandler) Succeeded() {}

// sortDNSSets sorts the DNSSet by name and then by record type.
func sortDNSSets(sets dns.DNSSets) dns.DNSSets {
	sortedSets := make(dns.DNSSets, len(sets))
	keys := make([]dns.DNSSetName, len(sets))
	for k := range sets {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		return keys[i].DNSName < keys[j].DNSName
	})

	for _, k := range keys {
		sortedSets[k] = sets[k]
	}

	return sortedSets
}

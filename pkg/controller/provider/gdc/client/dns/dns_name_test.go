// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package dns

import "testing"

func TestGetDNSRecordSetName(t *testing.T) {
	// Define a series of test cases in a "table"
	testCases := []struct {
		name       string // Name of the test case
		recordType string // Input recordType
		dnsName    string // Input DNS name
		want       string // Expected output
	}{
		// --- Basic Functionality ---
		{
			name:       "standard a record",
			recordType: "A",
			dnsName:    "mydns.org",
			want:       "a-mydns.org",
		},
		// --- Transformation Tests ---
		{
			name:       "uppercase record type",
			recordType: "CNAME",
			dnsName:    "sub.domain.com",
			want:       "cname-sub.domain.com",
		},
		{
			name:       "wildcard prefix",
			recordType: "TXT",
			dnsName:    "*.wildcard.org",
			want:       "txt-wildcard.org",
		},
		// --- Character Validation Tests ---
		{
			name:       "name with numbers is valid",
			recordType: "A",
			dnsName:    "3xample.goog1e.c0m",
			want:       "a-3xample.goog1e.c0m",
		},
		{
			name:       "name with uppercase letters",
			recordType: "A",
			dnsName:    "Example.gooGle.cOm",
			want:       "a-example.google.com",
		},
		{
			name:       "acme challenge with underscore",
			recordType: "TXT",
			dnsName:    "_acme-challenge.mydns.org",
			want:       "txt--acme-challenge.mydns.org",
		},
		{
			name:       "name with mixed invalid characters",
			recordType: "A",
			dnsName:    "a_b@c.com/!#d",
			// The regex replaces each invalid char or sequence with a single hyphen.
			want: "a-a-b-c.com-d",
		},
		// --- Test cases for wildcard ingress domains ---
		{
			name:       "sample 1 name with *",
			recordType: "A",
			dnsName:    "*.ingress.dev.gdc.gardener.cloud.sap",
			want:       "a-ingress.dev.gdc.gardener.cloud.sap",
		},
		{
			name:       "sample 2 name with *",
			recordType: "A",
			dnsName:    "*.ingress.orchestration.dog1-orc-hc-dev-gdc.dev.gdc.hc-poc.hanacloud.ondemand.com",
			want:       "a-ingress.orchestration.dog1-orc-hc-dev-gdc.dev.gdc.hc-poc.hanacloud.ondemand.com",
		},
		{
			name:       "name creating an invalid leading hyphen",
			recordType: "A",
			dnsName:    "*.-ingress.orchestration.com",
			want:       "a--ingress.orchestration.com",
		},
	}

	// Iterate over the test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := GetDNSRecordSetName(tc.recordType, tc.dnsName)

			if got != tc.want {
				t.Errorf("GetDNSRecordSetName(%q, %q) = %q; want %q", tc.recordType, tc.dnsName, got, tc.want)
			}
		})
	}
}

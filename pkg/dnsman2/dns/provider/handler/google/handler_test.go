/*
 * // SPDX-FileCopyrightText: Contributors to the Gardener project
 * //
 * // SPDX-License-Identifier: Apache-2.0
 */

package google

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ValidateServiceAccountJSON", func() {
	validJSON := `{
		"project_id": "valid-project-id",
		"type": "service_account"
	}`
	missingProjectIDJSON := `{
		"type": "service_account"
	}`
	invalidProjectID := `{
		"project_id": "(p)",
		"type": "service_account"
	}`
	invalidFormatJSON := `{
		"project_id": 12345,
		"type": "service_account"
	}`
	invalidTypeJSON := `{
		"project_id": "valid-project-id",
		"type": "invalid_type"
	}`
	missingTypeJSON := `{
		"project_id": "valid-project-id"
	}`
	allowedTokenURIJSON := `{
		"project_id": "valid-project-id",
		"type": "service_account",
		"token_uri": "https://oauth2.googleapis.com/token"
	}`
	forbiddenTokenURIJSON := `{
		"project_id": "valid-project-id",
		"type": "service_account",
		"token_uri": "http://169.254.169.254/computeMetadata/v1/instance/service-accounts/default/token"
	}`

	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{"Valid JSON", validJSON, false},
		{"Missing Project ID", missingProjectIDJSON, true},
		{"Invalid Project ID", invalidProjectID, true},
		{"Invalid Format", invalidFormatJSON, true},
		{"Invalid Type", invalidTypeJSON, true},
		{"Missing Type", missingTypeJSON, true},
		{"Allowed token_uri", allowedTokenURIJSON, false},
		{"Forbidden token_uri", forbiddenTokenURIJSON, true},
	}

	for _, tt := range tests {
		It(tt.name, func() {
			_, err := validateServiceAccountJSON([]byte(tt.data))
			if tt.wantErr {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})
	}
})

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package auth

// ServiceAccount config of GDC service identity private key.
type ServiceAccount struct {
	// Name is the Service account name. It will be used as the `iss` and `sub` claim in the self signed JWT.
	Name string `json:"name"`

	// Project ID associated with the service account credential.
	Project string `json:"project"`

	// The OAuth 2.0 Token URI.
	TokenURI string `json:"token_uri"`

	// ID of private key
	PrivateKeyID string `json:"private_key_id"`

	// Private key plain text content
	PrivateKey string `json:"private_key"`
}

// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package client

// OrgClusterConfig contains settings for an org cluster on GDC.
type OrgClusterConfig struct {
	// The URL to the org cluster.
	OrgClusterURL string `json:"orgClusterURL"`

	// Base64 encoded CA certificate.
	CAData string `json:"caData"`
}

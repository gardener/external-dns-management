// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package rfc2136

const (
	// PropertyServer is the secret data key for the authoritative server.
	PropertyServer = "Server"
	// PropertyZone is the secret data key for the zone.
	PropertyZone = "Zone"
	// PropertyTSIGKeyName is the secret data key for the key name of the TSIG secret (must end with a dot).
	PropertyTSIGKeyName = "TSIGKeyName"
	// PropertyTSIGSecret is the secret data key for TSIG secret.
	PropertyTSIGSecret = "TSIGSecret"
	// PropertyTSIGSecretAlgorithm is the secret data key for the TSIG Algorithm Name for Hash-based Message Authentication (HMAC).
	PropertyTSIGSecretAlgorithm = "TSIGSecretAlgorithm"
)

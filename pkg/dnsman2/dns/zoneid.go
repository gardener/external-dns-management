// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

// ZoneID represents a unique identifier for a DNS zone.
type ZoneID struct {
	ProviderType string
	ID           string
}

// NewZoneID creates a new ZoneID with the given provider type and zone ID.
func NewZoneID(providerType, zoneid string) ZoneID {
	return ZoneID{ProviderType: providerType, ID: zoneid}
}

func (z ZoneID) String() string {
	return z.ProviderType + "/" + z.ID
}

// IsEmpty returns true if the ZoneID is empty.
func (z ZoneID) IsEmpty() bool {
	return len(z.ProviderType)+len(z.ID) == 0
}

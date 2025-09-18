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

// ZoneInfo holds information about a DNS zone, including its ID, whether it is private, and its domain.
type ZoneInfo struct {
	zoneID  ZoneID
	private bool
	domain  string
	key     string // provider specific key
}

// NewZoneInfo creates a new ZoneInfo with the given parameters.
func NewZoneInfo(zoneID ZoneID, domain string, private bool, key string) ZoneInfo {
	return ZoneInfo{zoneID: zoneID, domain: domain, private: private, key: key}
}

func (zi ZoneInfo) String() string {
	return zi.zoneID.String() + " (" + zi.domain + ")"
}

// ZoneID returns the unique ID of the hosted zone.
func (zi ZoneInfo) ZoneID() ZoneID {
	return zi.zoneID
}

// Domain returns the domain of the hosted zone.
func (zi ZoneInfo) Domain() string {
	return zi.domain
}

// Key returns the provider specific key of the hosted zone.
func (zi ZoneInfo) Key() string {
	return zi.key
}

// IsPrivate returns true if the hosted zone is private.
func (zi ZoneInfo) IsPrivate() bool {
	return zi.private
}

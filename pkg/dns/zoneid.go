// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

type ZoneID struct {
	ProviderType string
	ID           string
}

func NewZoneID(providerType, zoneid string) ZoneID {
	return ZoneID{ProviderType: providerType, ID: zoneid}
}

func (z ZoneID) String() string {
	return z.ProviderType + "/" + z.ID
}

func (z ZoneID) IsEmpty() bool {
	return len(z.ProviderType)+len(z.ID) == 0
}

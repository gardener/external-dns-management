// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"
)

func TestLookupHost(t *testing.T) {
	c := createDNSClient("8.8.8.8")

	addrs, err := c.LookupHost("google-public-dns-a.google.com")
	if err != nil {
		t.Error("Error on LookupHost")
	}
	if len(addrs) != 1 {
		t.Error("Wrong count of results")
	}
	if addrs[0] != "8.8.8.8" {
		t.Errorf("Wrong address: %s != 8.8.8.8", addrs[0])
	}
}

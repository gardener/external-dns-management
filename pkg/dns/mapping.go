// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"strings"
)

func AlignHostname(host string) string {
	if strings.HasSuffix(host, ".") {
		return host
	}
	return host + "."
}

func NormalizeHostname(host string) string {
	if strings.HasPrefix(host, "\\052.") {
		host = "*" + host[4:]
	}
	if strings.HasSuffix(host, ".") {
		return host[:len(host)-1]
	}
	return host
}

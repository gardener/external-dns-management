// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"strings"
)

// EnsureTrailingDot ensures that the domain name has a trailing dot.
func EnsureTrailingDot(domainName string) string {
	if strings.HasSuffix(domainName, ".") {
		return domainName
	}
	return domainName + "."
}

// NormalizeDomainName normalizes the domain name, removing the trailing dot if present and replacing the escaped asterisk with a wildcard.
func NormalizeDomainName(domainName string) string {
	if strings.HasPrefix(domainName, "\\052.") {
		domainName = "*" + domainName[4:]
	}
	return strings.TrimSuffix(strings.ToLower(domainName), ".")
}

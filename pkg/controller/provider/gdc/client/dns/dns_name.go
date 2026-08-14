// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"fmt"
	"regexp"
	"strings"
)

// invalidCharsRegex is a regular expression that matches one or more consecutive characters
// that are not a lowercase letter, a number, a hyphen, or a period.
var invalidCharsRegex = regexp.MustCompile(`[^a-z0-9-.]+`)

// GetDNSRecordSetName generates a valid DNS ResourceRecordSet name from a record type and a DNS name.
//
// Cloud DNS providers often have strict validation rules for ResourceRecordSet names
// (e.g., must match regex "[^a-z0-9-.]+").
// This function transforms a Gardener DNS record name into a compliant format by applying several rules.
//
// The transformations are:
//  1. The lowercase record type is prepended to the name, followed by a hyphen.
//  2. A leading wildcard prefix "*." is trimmed from the DNS name.
//  3. Any invalid characters (like the underscore in "_acme-challenge") are replaced with a hyphen.
//
// For example:
//   - ("A", "mydns.org") -> "a-mydns.org"
//   - ("TXT", "*.wildcard.org") -> "txt-wildcard.org"
//   - ("TXT", "_acme-challenge.mydns.org") -> "txt--acme-challenge.mydns.org"
//
// Note: Since this transformation can cause different inputs to map to the same output,
// the caller is responsible for handling potential name collisions.
func GetDNSRecordSetName(recordType string, name string) string {
	objName := fmt.Sprintf("%s-%s", recordType, strings.TrimPrefix(name, "*."))
	return invalidCharsRegex.ReplaceAllString(strings.ToLower(objName), "-")
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

func SupportedRecordType(t string) bool {
	switch t {
	case RS_CNAME, RS_A, RS_AAAA, RS_TXT:
		return true
	}
	return false
}

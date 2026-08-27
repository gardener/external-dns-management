// SPDX-FileCopyrightText: Contributors to the Gardener project
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

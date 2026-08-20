// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gdc

const (
	RS_A     = "A"
	RS_TXT   = "TXT"
	RS_CNAME = "CNAME"
	RS_PTR   = "PTR"
	RS_DS    = "DS"
	RS_MX    = "MX"
)

var supportedRecordTypes = []string{RS_A, RS_TXT, RS_CNAME, RS_PTR, RS_DS, RS_MX}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

const InvalidToken = "[invalid token]"

const (
	// ProtocolVersion0 without support for routing policy
	ProtocolVersion0 = 0
	// ProtocolVersion1 with support for routing policy
	ProtocolVersion1 = 1
)

type DNSSets map[string]*DNSSet

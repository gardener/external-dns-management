// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"github.com/gardener/external-dns-management/pkg/dns"
)

type TargetProvider interface {
	Targets() Targets
	TTL() int64
	OwnerId() string
	RoutingPolicy() *dns.RoutingPolicy
}

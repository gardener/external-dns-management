// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"net"

	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

type (
	Target  = dnsutils.Target
	Targets = dnsutils.Targets
)

func NewHostTargetFromEntryVersion(name string, entry *EntryVersion) (Target, error) {
	ip := net.ParseIP(name)
	if ip == nil {
		return dnsutils.NewTargetWithIPStack(dns.RS_CNAME, name, entry.TTL(), entry.GetAnnotations()[dns.AnnotationIPStack]), nil
	} else if ip.To4() != nil {
		return dnsutils.NewTarget(dns.RS_A, name, entry.TTL()), nil
	} else if ip.To16() != nil {
		return dnsutils.NewTarget(dns.RS_AAAA, name, entry.TTL()), nil
	} else {
		return nil, fmt.Errorf("unexpected IP address (never ipv4 or ipv6): %s (%s)", ip.String(), name)
	}
}

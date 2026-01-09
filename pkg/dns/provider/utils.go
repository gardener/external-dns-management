// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"maps"
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/resources"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
)

type NullMetrics struct{}

var _ Metrics = &NullMetrics{}

func (m *NullMetrics) AddGenericRequests(_ string, _ int) {
}

func (m *NullMetrics) AddZoneRequests(_, _ string, _ int) {
}

func copyZones(src map[dns.ZoneID]*dnsHostedZone) dnsHostedZones {
	dst := dnsHostedZones{}
	maps.Copy(dst, src)
	return dst
}

func errorValue(format string, err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf(format, err.Error())
}

func filterZoneByProvider(zones []*dnsHostedZone, provider DNSProvider) *dnsHostedZone {
	if provider != nil {
		for _, zone := range zones {
			if provider.IncludesZone(zone.Id()) {
				return zone
			}
		}
	}
	return nil
}

func assureRateLimit(mod *resources.ModificationState, t **api.RateLimit, s *api.RateLimit) {
	if s == nil && *t != nil {
		*t = nil
		mod.Modify(true)
	} else if s != nil {
		if *t == nil || !reflect.DeepEqual(**t, *s) {
			*t = s
			mod.Modify(true)
		}
	}
}

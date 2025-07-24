// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	miekgdns "github.com/miekg/dns"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// NameserversProvider is a function that returns the current nameservers to use for DNS queries.
type NameserversProvider interface {
	// Nameservers returns the current nameservers to use for DNS queries.
	Nameservers(ctx context.Context) ([]string, error)
}

var defaultNameservers = []string{
	"8.8.8.8:53",
	"8.8.4.4:53",
}

// SystemNameservers is the NameserversProvider that returns the system's nameservers or falls back to the defaults.
var SystemNameservers = &systemNameserversProvider{}

// systemNameserversProvider is a NameserversProvider that returns the system's nameservers or falls back to the defaults.
type systemNameserversProvider struct {
	once        sync.Once
	nameservers []string
}

// Nameservers returns the current nameservers to use for DNS queries.
func (s *systemNameserversProvider) Nameservers(_ context.Context) ([]string, error) {
	s.once.Do(func() {
		s.nameservers = getSystemNameservers("/etc/resolv.conf", defaultNameservers)
	})
	return s.nameservers, nil
}

// getSystemNameservers attempts to get systems nameservers before falling back to the defaults
func getSystemNameservers(path string, defaults []string) []string {
	config, err := miekgdns.ClientConfigFromFile(path)
	if err != nil || len(config.Servers) == 0 {
		return defaults
	}

	systemNameservers := []string{}
	for _, server := range config.Servers {
		// ensure all servers have a port number
		if _, _, err := net.SplitHostPort(server); err != nil {
			systemNameservers = append(systemNameservers, net.JoinHostPort(server, "53"))
		} else {
			systemNameservers = append(systemNameservers, server)
		}
	}
	return systemNameservers
}

// HostedZoneNameserversProvider provides nameservers for a specific DNS hosted zone.
type HostedZoneNameserversProvider struct {
	lock              sync.Mutex
	fqdnZone          string
	nextUpdate        time.Time
	minRefreshPeriod  time.Duration
	nameservers       []string
	systemNameservers NameserversProvider
}

// NewHostedZoneNameserversProvider creates a new HostedZoneNameserversProvider.
func NewHostedZoneNameserversProvider(ctx context.Context, fqdnZone string, minRefreshPeriod time.Duration, systemNameservers NameserversProvider) (*HostedZoneNameserversProvider, error) {
	instance := &HostedZoneNameserversProvider{
		fqdnZone:          dns.EnsureTrailingDot(fqdnZone),
		minRefreshPeriod:  minRefreshPeriod,
		systemNameservers: systemNameservers,
	}

	if servers, err := instance.Nameservers(ctx); err != nil {
		return nil, err
	} else if len(servers) == 0 {
		return nil, fmt.Errorf("no nameservers found for zone %s", fqdnZone)
	}

	return instance, nil
}

// Nameservers returns the current nameservers to use for DNS queries.
func (h *HostedZoneNameserversProvider) Nameservers(ctx context.Context) ([]string, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	if time.Now().After(h.nextUpdate) {
		ns, ttl, err := h.retrieveNameservers(ctx)
		if err != nil {
			return nil, err
		}
		h.nameservers = ns
		h.nextUpdate = time.Now().Add(min(h.minRefreshPeriod, time.Duration(ttl)*time.Second))
	}

	return h.nameservers, nil
}

func (h *HostedZoneNameserversProvider) retrieveNameservers(ctx context.Context) ([]string, int64, error) {
	queryDNS := NewStandardQueryDNS(h.systemNameservers)
	result := queryDNS.Query(ctx, dns.DNSSetName{DNSName: h.fqdnZone}.Normalize(), dns.TypeNS)
	if result.Err != nil || result.RecordSet == nil || len(result.RecordSet.Records) == 0 {
		return nil, 0, result.Err
	}
	var nameservers []string
	for _, record := range result.RecordSet.Records {
		nameservers = append(nameservers, record.Value+":53") // Ensure all nameservers have a port number
	}
	return nameservers, result.RecordSet.TTL, nil
}

/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. h file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package rfc2136

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
	miekgdns "github.com/miekg/dns"
)

const (
	// maximum time DNS client can be off from server for an update to succeed
	clockSkew = 300
	tcp       = "tcp"
	// recreateOnTTLChange specifies if changing TTL of a recordset needs deletion and insert of recordset
	recreateOnTTLChange = true
)

type Handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig
	cache  provider.ZoneCache
	ctx    context.Context

	nameserver    string
	zone          string
	tsigKeyname   string
	tsigSecret    string
	tsigAlgorithm string
}

var _ provider.DNSHandler = &Handler{}

// tsigAlgs are the supported TSIG algorithms
var tsigAlgs = []string{miekgdns.HmacSHA1, miekgdns.HmacSHA224, miekgdns.HmacSHA256, miekgdns.HmacSHA384, miekgdns.HmacSHA512}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *c,
	}

	h.ctx = c.Context

	server, err := c.GetRequiredProperty("Server")
	if err != nil {
		return nil, err
	}
	if !strings.Contains(server, ":") {
		server += ":53"
	}
	h.nameserver = server

	zone, err := c.GetRequiredProperty("Zone")
	if err != nil {
		return nil, err
	}
	if zone != miekgdns.CanonicalName(zone) {
		return nil, fmt.Errorf("zone must be given in canonical form: '%s' instead of '%s'", miekgdns.CanonicalName(zone), h.zone)
	}
	h.zone = zone

	keyname, err := c.GetRequiredProperty("TSIGKeyName")
	if err != nil {
		return nil, err
	}
	if keyname != miekgdns.Fqdn(keyname) {
		return nil, fmt.Errorf("TSIGKeyName must end with '.'")
	}
	h.tsigKeyname = miekgdns.Fqdn(keyname)

	secret, err := c.GetRequiredProperty("TSIGSecret")
	if err != nil {
		return nil, err
	}
	h.tsigSecret = secret

	h.tsigAlgorithm, err = findTsigAlgorithm(c.GetProperty("TSIGSecretAlgorithm"))
	if err != nil {
		return nil, err
	}

	h.cache, err = c.ZoneCacheFactory.CreateZoneCache(provider.CacheZoneState, c.Metrics, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func (h *Handler) Release() {
	h.cache.Release()
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) getZones(cache provider.ZoneCache) (provider.DNSHostedZones, error) {
	domainName := dns.NormalizeHostname(h.zone)
	h.config.Metrics.AddGenericRequests(provider.M_LISTZONES, 1)
	return provider.DNSHostedZones{
		provider.NewDNSHostedZone(TYPE_CODE, h.zone, domainName, h.zone, false),
	}, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	if zone.Id().ID != h.zone {
		return nil, fmt.Errorf("Handler supports only zone %s", h.zone)
	}

	msg := &miekgdns.Msg{}
	msg.SetAxfr(h.zone)
	msg.SetTsig(h.tsigKeyname, h.tsigAlgorithm, clockSkew, time.Now().Unix())

	h.config.RateLimiter.Accept()
	t := &miekgdns.Transfer{
		TsigSecret: map[string]string{h.tsigKeyname: h.tsigSecret},
	}
	env, err := t.In(msg, h.nameserver)
	if err != nil {
		return nil, fmt.Errorf("axfr request failed: %w", err)
	}

	for e := range env {
		h.config.Metrics.AddZoneRequests(zone.Id().ID, provider.M_LISTRECORDS, 1)
		if e.Error != nil {
			if e.Error == miekgdns.ErrSoa {
				return nil, fmt.Errorf("AXFR error: unexpected response received from the server")
			} else {
				return nil, fmt.Errorf("AXFR error: %v", e.Error)
			}
		}

		var (
			lastRS   *dns.RecordSet
			lastName string
			lastType uint16
		)
		for _, rr := range e.RR {
			fullName := dns.NormalizeHostname(rr.Header().Name)
			if lastRS != nil && (fullName != lastName || lastType != rr.Header().Rrtype) {
				dnssets.AddRecordSetFromProvider(lastName, lastRS)
				lastRS = nil
			}
			var (
				currentRType  string
				currentRecord *dns.Record
			)
			switch rr.Header().Rrtype {
			case miekgdns.TypeA:
				currentRType = dns.RS_A
				currentRecord = &dns.Record{Value: rr.(*miekgdns.A).A.String()}
			case miekgdns.TypeAAAA:
				currentRType = dns.RS_AAAA
				currentRecord = &dns.Record{Value: rr.(*miekgdns.AAAA).AAAA.String()}
			case miekgdns.TypeCNAME:
				currentRType = dns.RS_CNAME
				currentRecord = &dns.Record{Value: rr.(*miekgdns.CNAME).Target}
			case miekgdns.TypeTXT:
				txt := rr.(*miekgdns.TXT).Txt
				records := make([]*dns.Record, len(txt))
				for i, t := range txt {
					records[i] = &dns.Record{Value: raw.EnsureQuotedText(t)}
				}
				dnssets.AddRecordSetFromProvider(fullName, dns.NewRecordSet(dns.RS_TXT, int64(rr.Header().Ttl), records))
			default:
			}
			lastName = fullName
			lastType = rr.Header().Rrtype
			if currentRecord != nil {
				if lastRS != nil {
					lastRS.Records = append(lastRS.Records, currentRecord)
				} else {
					lastRS = dns.NewRecordSet(currentRType, int64(rr.Header().Ttl), []*dns.Record{currentRecord})
				}
			}
		}
		if lastRS != nil {
			dnssets.AddRecordSetFromProvider(lastName, lastRS)
		}
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ReportZoneStateConflict(zone provider.DNSHostedZone, err error) bool {
	return h.cache.ReportZoneStateConflict(zone, err)
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	exec := NewExecution(logger, h)

	var succeeded, failed int
	for _, r := range reqs {
		inserts, deletes, err := exec.buildRecordSet(r)
		if err != nil {
			if r.Done != nil {
				r.Done.SetInvalid(err)
			}
			continue
		}

		err = exec.apply(inserts, deletes, h.config.Metrics)
		if err != nil {
			failed++
			logger.Infof("Apply failed with %s", err.Error())
			if r.Done != nil {
				r.Done.Failed(err)
			}
		} else {
			succeeded++
			if r.Done != nil {
				r.Done.Succeeded()
			}
		}
	}

	if succeeded > 0 {
		logger.Infof("Succeeded updates for records in zone %s: %d", zone.Id(), succeeded)
	}
	if failed > 0 {
		logger.Infof("Failed updates for records in zone %s: %d", zone.Id(), failed)
		return fmt.Errorf("%d changes failed", failed)
	}

	return nil
}

func findTsigAlgorithm(alg string) (string, error) {
	if alg == "" {
		return miekgdns.HmacSHA256, nil
	}

	fqdnAlg := miekgdns.Fqdn(alg)
	for _, a := range tsigAlgs {
		if fqdnAlg == a {
			return fqdnAlg, nil
		}
	}
	return "", fmt.Errorf("invalid TSIG secret algorithm: %s (supported: %s)", alg, strings.ReplaceAll(strings.Join(tsigAlgs, ","), ".", ""))
}

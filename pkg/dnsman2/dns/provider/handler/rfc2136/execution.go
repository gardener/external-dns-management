// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package rfc2136

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	miekgdns "github.com/miekg/dns"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type execution struct {
	log     logr.Logger
	handler *handler
	zone    provider.DNSHostedZone
}

func newExecution(log logr.Logger, h *handler, zone provider.DNSHostedZone) *execution {
	return &execution{log: log, handler: h, zone: zone}
}

func (exec *execution) apply(_ context.Context, setName dns.DNSSetName, req *provider.ChangeRequestUpdate) error {
	var (
		err                error
		addRRSet, delRRSet []miekgdns.RR
		action             string
	)

	domain := dns.NormalizeDomainName(exec.handler.zone)
	if req.New != nil {
		action = "CREATE"
		if req.Old != nil {
			action = "UPDATE"
		}
		exec.log.Info(fmt.Sprintf("Desired %s: %s record set %s[%s] with TTL %d: %s", action, req.New.Type, setName.DNSName, domain, req.New.TTL, req.New.RecordString()))
		addRRSet, err = exec.buildResourceRecords(setName, req.New)
		if err != nil {
			return err
		}
		if req.Old != nil {
			newRecords, updRecords, delRecords := req.New.DiffTo(req.Old)
			addRecords := append(newRecords, updRecords...)
			if recreateOnTTLChange {
				// experience with know-dns: update of TTL needs deletion and insert
				delRecords = append(delRecords, updRecords...)
			}
			addRRSet, err = exec.buildResourceRecords(setName, &dns.RecordSet{Type: req.New.Type, TTL: req.New.TTL, Records: addRecords})
			if err != nil {
				return err
			}
			if len(delRecords) > 0 {
				delRRSet, err = exec.buildResourceRecords(setName, &dns.RecordSet{Type: req.Old.Type, TTL: req.Old.TTL, Records: delRecords})
				if err != nil {
					return err
				}
			}
		}
	} else if req.Old != nil {
		exec.log.Info(fmt.Sprintf("Desired DELETE: %s record set %s[%s] with TTL %d: %s", req.Old.Type, setName.DNSName, domain, req.Old.TTL, req.Old.RecordString()))
		delRRSet, err = exec.buildResourceRecords(setName, req.Old)
		if err != nil {
			return err
		}
	}

	if len(delRRSet) != 0 {
		if err := exec.delete(delRRSet); err != nil {
			return err
		}
	}
	if len(addRRSet) != 0 {
		if err := exec.insert(addRRSet); err != nil {
			return err
		}
	}
	return nil
}

func (exec *execution) buildResourceRecords(setName dns.DNSSetName, rset *dns.RecordSet) ([]miekgdns.RR, error) {
	if setName.SetIdentifier != "" || rset.RoutingPolicy != nil {
		return nil, fmt.Errorf("routing policies not supported for " + ProviderType)
	}

	var records []miekgdns.RR

	hdr := miekgdns.RR_Header{
		Name: miekgdns.Fqdn(setName.DNSName),
		Ttl:  utils.TTLToUint32(rset.TTL),
	}
	switch rset.Type {
	case dns.TypeA:
		hdr.Rrtype = miekgdns.TypeA
		for _, r := range rset.Records {
			ip := net.ParseIP(r.Value)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP address: %s", r.Value)
			}
			if ip.To4() == nil {
				return nil, fmt.Errorf("not an IPv4 address: %s", r.Value)
			}
			records = append(records, &miekgdns.A{Hdr: hdr, A: ip.To4()})
		}
		return records, nil
	case dns.TypeAAAA:
		hdr.Rrtype = miekgdns.TypeAAAA
		for _, r := range rset.Records {
			ip := net.ParseIP(r.Value)
			if ip == nil {
				return nil, fmt.Errorf("invalid IP address: %s", r.Value)
			}
			if ip.To16() == nil {
				return nil, fmt.Errorf("not an IPv6 address: %s", r.Value)
			}
			records = append(records, &miekgdns.AAAA{Hdr: hdr, AAAA: ip.To16()})
		}
		return records, nil
	case dns.TypeCNAME:
		hdr.Rrtype = miekgdns.TypeCNAME
		for _, r := range rset.Records {
			records = append(records, &miekgdns.CNAME{Hdr: hdr, Target: miekgdns.Fqdn(r.Value)})
		}
		return records, nil
	case dns.TypeTXT:
		hdr.Rrtype = miekgdns.TypeTXT
		txtRecord := &miekgdns.TXT{Hdr: hdr}
		for _, r := range rset.Records {
			unquoted, err := strconv.Unquote(r.Value)
			if err != nil {
				unquoted = r.Value
			}
			txtRecord.Txt = append(txtRecord.Txt, unquoted)
		}
		records = append(records, txtRecord)
		return records, nil
	default:
		return nil, fmt.Errorf("unexpected record type: %s", rset.Type)
	}
}

func (exec *execution) insert(records []miekgdns.RR) error {
	return exec.exchange(records,
		func(msg *miekgdns.Msg, rrs []miekgdns.RR) { msg.Insert(rrs) },
		provider.MetricsRequestTypeUpdateRecords,
		exec.handler.config.Metrics)
}

func (exec *execution) delete(records []miekgdns.RR) error {
	return exec.exchange(records,
		func(msg *miekgdns.Msg, rrs []miekgdns.RR) { msg.Remove(rrs) },
		provider.MetricsRequestTypeDeleteRecords,
		exec.handler.config.Metrics)
}

func (exec *execution) exchange(records []miekgdns.RR, apply func(*miekgdns.Msg, []miekgdns.RR), requestType provider.MetricsRequestType, metrics provider.Metrics) error {
	exec.handler.config.RateLimiter.Accept()

	c := &miekgdns.Client{
		Net:        tcp,
		TsigSecret: map[string]string{exec.handler.tsigKeyname: exec.handler.tsigSecret},
	}

	msg := &miekgdns.Msg{}
	msg.SetUpdate(exec.handler.zone)
	msg.SetTsig(exec.handler.tsigKeyname, exec.handler.tsigAlgorithm, clockSkew, time.Now().Unix())

	apply(msg, records)

	retMsg, _, err := c.Exchange(msg, exec.handler.nameserver) // your DNS server
	metrics.AddZoneRequests(exec.handler.zone, requestType, 1)
	if err != nil {
		return err
	}

	if retMsg.Rcode != miekgdns.RcodeSuccess {
		return fmt.Errorf("DNS server returned error code on %s: %d", requestType, retMsg.Rcode)
	}
	return nil
}

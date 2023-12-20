/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. exec file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use exec file except in compliance with the License.
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
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	miekgdns "github.com/miekg/dns"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

type Change struct {
	Done provider.DoneHandler
}

type Execution struct {
	logger.LogContext
	handler *Handler

	changes map[string][]*Change
}

func NewExecution(logger logger.LogContext, h *Handler) *Execution {
	return &Execution{LogContext: logger, handler: h, changes: map[string][]*Change{}}
}

func (exec *Execution) buildRecordSet(req *provider.ChangeRequest) ([]miekgdns.RR, []miekgdns.RR, error) {
	var setName dns.DNSSetName
	var addset, delset *dns.RecordSet

	domain := dns.NormalizeHostname(exec.handler.zone)
	if req.Addition != nil {
		setName, addset = dns.MapToProvider(req.Type, req.Addition, domain)
	}
	if req.Deletion != nil {
		setName, delset = dns.MapToProvider(req.Type, req.Deletion, domain)
	}
	if setName.DNSName == "" || (addset.Length() == 0 && delset.Length() == 0) {
		return nil, nil, nil
	}

	if setName.SetIdentifier != "" || req.Addition != nil && req.Addition.RoutingPolicy != nil || req.Deletion != nil && req.Deletion.RoutingPolicy != nil {
		return nil, nil, fmt.Errorf("routing policies not supported for " + TYPE_CODE)
	}

	var err error
	var addRRSet, delRRSet []miekgdns.RR
	if req.Addition != nil {
		exec.Infof("Desired %s: %s record set %s[%s] with TTL %d: %s", req.Action, addset.Type, setName.DNSName, domain, addset.TTL, addset.RecordString())
		addRRSet, err = exec.buildMappedRecordSet(setName.DNSName, addset)
		if err != nil {
			return nil, nil, err
		}
		if req.Deletion != nil {
			newRecords, updRecords, delRecords := addset.DiffTo(delset)
			addRecords := append(newRecords, updRecords...)
			if recreateOnTTLChange {
				// experience with know-dns: update of TTL needs deletion and insert
				delRecords = append(delRecords, updRecords...)
			}
			addRRSet, err = exec.buildMappedRecordSet(setName.DNSName, &dns.RecordSet{Type: addset.Type, TTL: addset.TTL, Records: addRecords})
			if err != nil {
				return nil, nil, err
			}
			if len(delRecords) > 0 {
				delRRSet, err = exec.buildMappedRecordSet(setName.DNSName, &dns.RecordSet{Type: delset.Type, TTL: delset.TTL, Records: delRecords})
				if err != nil {
					return nil, nil, err
				}
			}
		}
	} else {
		exec.Infof("Desired %s: %s record set %s[%s] with TTL %d: %s", req.Action, delset.Type, setName.DNSName, domain, delset.TTL, delset.RecordString())
		delRRSet, err = exec.buildMappedRecordSet(setName.DNSName, delset)
		if err != nil {
			return nil, nil, err
		}
	}

	return addRRSet, delRRSet, nil
}

func (exec *Execution) buildMappedRecordSet(name string, rset *dns.RecordSet) ([]miekgdns.RR, error) {
	var records []miekgdns.RR

	hdr := miekgdns.RR_Header{
		Name: miekgdns.Fqdn(name),
		Ttl:  uint32(rset.TTL),
	}
	switch rset.Type {
	case dns.RS_A:
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
	case dns.RS_AAAA:
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
	case dns.RS_CNAME:
		hdr.Rrtype = miekgdns.TypeCNAME
		for _, r := range rset.Records {
			records = append(records, &miekgdns.CNAME{Hdr: hdr, Target: miekgdns.Fqdn(r.Value)})
		}
		return records, nil
	case dns.RS_TXT:
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

func (exec *Execution) apply(inserts, deletes []miekgdns.RR, metrics provider.Metrics) error {
	if len(deletes) != 0 {
		if err := exec.delete(deletes, metrics); err != nil {
			return err
		}
	}
	if len(inserts) != 0 {
		if err := exec.insert(inserts, metrics); err != nil {
			return err
		}
	}
	return nil
}

func (exec *Execution) insert(records []miekgdns.RR, metrics provider.Metrics) error {
	return exec.exchange(records, func(msg *miekgdns.Msg, rrs []miekgdns.RR) { msg.Insert(rrs) }, provider.M_UPDATERECORDS, metrics)
}

func (exec *Execution) delete(records []miekgdns.RR, metrics provider.Metrics) error {
	return exec.exchange(records, func(msg *miekgdns.Msg, rrs []miekgdns.RR) { msg.Remove(rrs) }, provider.M_DELETERECORDS, metrics)
}

func (exec *Execution) exchange(records []miekgdns.RR, apply func(*miekgdns.Msg, []miekgdns.RR), requestType string, metrics provider.Metrics) error {
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

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	miekgdns "github.com/miekg/dns"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

func ToFQDN(domainName string) string {
	if strings.HasSuffix(domainName, ".") {
		return domainName
	}
	return domainName + "."
}

// QueryDNSResult contains the result of a DNS query.
type QueryDNSResult struct {
	// Records contains the DNS record set.
	Records []dns.Record
	// TTL is the time-to-live of the record set.
	TTL uint32
	//	Err is the error that occurred during the query.
	Err error
}

// QueryDNS is an interface for querying DNS records.
type QueryDNS interface {
	// Query returns the DNS records for the given DNS name and record type.
	Query(ctx context.Context, dnsName string, rstype dns.RecordType) QueryDNSResult
}

type standardQueryDNS struct {
	nameservers NameserversProvider
	timeout     time.Duration
}

// NewStandardQueryDNS creates a new StandardQueryDNS.
func NewStandardQueryDNS(nameservers NameserversProvider) QueryDNS {
	return &standardQueryDNS{nameservers: nameservers, timeout: 10 * time.Second}
}

// NewStandardQueryDNSWithTimeout creates a new StandardQueryDNS with a custom DNS timeout.
func NewStandardQueryDNSWithTimeout(nameservers NameserversProvider, timeout time.Duration) QueryDNS {
	return &standardQueryDNS{nameservers: nameservers, timeout: timeout}
}

func (q *standardQueryDNS) Query(_ context.Context, dnsName string, rstype dns.RecordType) QueryDNSResult {
	var (
		result = QueryDNSResult{TTL: 30}
		rtype  uint16
	)
	switch rstype {
	case dns.TypeA:
		rtype = miekgdns.TypeA
	case dns.TypeAAAA:
		rtype = miekgdns.TypeAAAA
	case dns.TypeNS:
		rtype = miekgdns.TypeNS
	case dns.TypeCNAME:
		rtype = miekgdns.TypeCNAME
	case dns.TypeTXT:
		rtype = miekgdns.TypeTXT
	default:
		result.Err = fmt.Errorf("unsupported record type %s", rstype)
		return result
	}

	msg, err := q.dnsQuery(ToFQDN(dnsName), rtype)
	if err != nil {
		result.Err = err
		return result
	}
	if msg.Rcode != miekgdns.RcodeSuccess {
		result.Err = fmt.Errorf("DNS lookup failed with rcode %d", msg.Rcode)
		return result
	}
	addRecord := func(value string, ttl uint32) {
		if len(result.Records) == 0 || ttl < result.TTL {
			result.TTL = ttl
		}
		result.Records = append(result.Records, dns.Record{Value: value})
	}
	for _, rr := range msg.Answer {
		switch rstype {
		case dns.TypeA:
			r, ok := rr.(*miekgdns.A)
			if !ok {
				result.Err = fmt.Errorf("unexpected record type %T (A)", rr)
				return result
			}
			addRecord(r.A.String(), r.Hdr.Ttl)
		case dns.TypeAAAA:
			r, ok := rr.(*miekgdns.AAAA)
			if !ok {
				result.Err = fmt.Errorf("unexpected record type %T (AAAA)", rr)
				return result
			}
			addRecord(r.AAAA.String(), r.Hdr.Ttl)
		case dns.TypeTXT:
			r, ok := rr.(*miekgdns.TXT)
			if !ok {
				result.Err = fmt.Errorf("unexpected record type %T (TXT)", rr)
				return result
			}
			for _, txt := range r.Txt {
				addRecord(txt, r.Hdr.Ttl)
			}
		case dns.TypeNS:
			r, ok := rr.(*miekgdns.NS)
			if !ok {
				result.Err = fmt.Errorf("unexpected record type %T (NS)", rr)
				return result
			}
			addRecord(r.Ns, r.Hdr.Ttl)
		case dns.TypeCNAME:
			r, ok := rr.(*miekgdns.CNAME)
			if !ok {
				result.Err = fmt.Errorf("unexpected record type %T (CNAME)", rr)
				return result
			}
			addRecord(r.Target, r.Hdr.Ttl)
		}
	}
	return result
}

func (q *standardQueryDNS) dnsQuery(fqdn string, rtype uint16) (*miekgdns.Msg, error) {
	m := new(miekgdns.Msg)
	m.SetQuestion(fqdn, rtype)
	m.SetEdns0(4096, false)
	m.RecursionDesired = true

	nameservers, err := q.nameservers.Nameservers()
	if err != nil {
		return nil, fmt.Errorf("failed to get nameservers: %v", err)
	}

	var in *miekgdns.Msg
	for _, ns := range nameservers {
		in, err = q.sendDNSQuery(m, ns)
		if err == nil && in.Rcode == miekgdns.RcodeSuccess {
			break
		}
	}
	return in, err
}

func (q *standardQueryDNS) sendDNSQuery(m *miekgdns.Msg, ns string) (*miekgdns.Msg, error) {
	udp := &miekgdns.Client{Net: "udp", Timeout: q.timeout}
	in, _, err := udp.Exchange(m, ns)

	// customization: try TCP if UDP fails
	if err != nil || in != nil && in.Truncated {
		tcp := &miekgdns.Client{Net: "tcp", Timeout: q.timeout}
		// If the TCP request succeeds, the error will reset to nil
		var err2 error
		in, _, err2 = tcp.Exchange(m, ns)
		if err == nil {
			err = err2
		} else if err2 != nil {
			err = fmt.Errorf("DNS lookup: udp failed with %s, tcp failed with %s", err, err2)
		} else {
			err = nil
		}
	}

	return in, err
}

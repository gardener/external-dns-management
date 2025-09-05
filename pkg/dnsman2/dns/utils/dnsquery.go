// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	miekgdns "github.com/miekg/dns"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// ToFQDN returns the fully qualified domain name for the given domain name.
func ToFQDN(domainName string) string {
	if strings.HasSuffix(domainName, ".") {
		return domainName
	}
	return domainName + "."
}

// QueryDNSResult contains the result of a DNS query.
type QueryDNSResult struct {
	// RecordSet contains the DNS records returned by the query.
	RecordSet *dns.RecordSet
	//	Err is the error that occurred during the query.
	Err error
}

// QueryDNSFactoryFunc is a function that creates a QueryDNS instance.
type QueryDNSFactoryFunc func() (QueryDNS, error)

// QueryDNS is an interface for querying DNS records.
type QueryDNS interface {
	// Query returns the DNS records for the given DNS name and record type.
	Query(ctx context.Context, setName dns.DNSSetName, rstype dns.RecordType) QueryDNSResult
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

func (q *standardQueryDNS) Query(ctx context.Context, setName dns.DNSSetName, rstype dns.RecordType) QueryDNSResult {
	if setName.SetIdentifier != "" {
		return QueryDNSResult{Err: fmt.Errorf("set identifier is not supported for DNS queries")}
	}
	var (
		rtype uint16
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
		return QueryDNSResult{Err: fmt.Errorf("unsupported record type %s", rstype)}
	}

	msg, err := q.dnsQuery(ctx, ToFQDN(setName.DNSName), rtype)
	if err != nil {
		return QueryDNSResult{Err: err}
	}
	if msg.Rcode != miekgdns.RcodeSuccess {
		if msg.Rcode == miekgdns.RcodeNameError {
			return QueryDNSResult{} // NXDOMAIN is not an error, just no records
		}
		return QueryDNSResult{Err: fmt.Errorf("DNS lookup failed with rcode %d", msg.Rcode)}
	}
	rs := dns.NewRecordSet(rstype, 0, nil)
	addRecord := func(value string, ttl uint32) {
		if len(rs.Records) == 0 || int64(ttl) < rs.TTL {
			rs.TTL = int64(ttl)
		}
		rs.Add(&dns.Record{Value: value})
	}
	for _, rr := range msg.Answer {
		switch rstype {
		case dns.TypeA:
			r, ok := rr.(*miekgdns.A)
			if !ok {
				return QueryDNSResult{Err: fmt.Errorf("unexpected record type %T (A)", rr)}
			}
			addRecord(r.A.String(), r.Hdr.Ttl)
		case dns.TypeAAAA:
			r, ok := rr.(*miekgdns.AAAA)
			if !ok {
				return QueryDNSResult{Err: fmt.Errorf("unexpected record type %T (AAAA)", rr)}
			}
			addRecord(r.AAAA.String(), r.Hdr.Ttl)
		case dns.TypeTXT:
			r, ok := rr.(*miekgdns.TXT)
			if !ok {
				return QueryDNSResult{Err: fmt.Errorf("unexpected record type %T (TXT)", rr)}
			}
			for _, txt := range r.Txt {
				addRecord(txt, r.Hdr.Ttl)
			}
		case dns.TypeNS:
			r, ok := rr.(*miekgdns.NS)
			if !ok {
				return QueryDNSResult{Err: fmt.Errorf("unexpected record type %T (NS)", rr)}
			}
			addRecord(r.Ns, r.Hdr.Ttl)
		case dns.TypeCNAME:
			r, ok := rr.(*miekgdns.CNAME)
			if !ok {
				return QueryDNSResult{Err: fmt.Errorf("unexpected record type %T (CNAME)", rr)}
			}
			addRecord(dns.NormalizeDomainName(r.Target), r.Hdr.Ttl)
		default:
			return QueryDNSResult{Err: fmt.Errorf("unsupported record type %s in DNS response", rstype)}
		}
	}
	return QueryDNSResult{RecordSet: rs}
}

func (q *standardQueryDNS) dnsQuery(ctx context.Context, fqdn string, rtype uint16) (*miekgdns.Msg, error) {
	m := new(miekgdns.Msg)
	m.SetQuestion(fqdn, rtype)
	m.SetEdns0(4096, false)
	m.RecursionDesired = true

	nameservers, err := q.nameservers.Nameservers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get nameservers: %v", err)
	}

	var in *miekgdns.Msg
	for _, ns := range nameservers {
		in, err = q.sendDNSQuery(ctx, m, ns)
		if err == nil && in.Rcode == miekgdns.RcodeSuccess {
			break
		}
	}
	return in, err
}

func (q *standardQueryDNS) sendDNSQuery(ctx context.Context, m *miekgdns.Msg, ns string) (*miekgdns.Msg, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("standardQueryDNS")

	udp := &miekgdns.Client{Net: "udp", Timeout: q.timeout}
	in, _, err := udp.Exchange(m, ns)
	if err != nil {
		log.V(1).Error(err, "DNS query failed", "nameserver", ns, "message", m, "timeout", q.timeout)
	} else {
		log.V(1).Info("DNS query succeeded", "nameserver", ns, "message", in, "timeout", q.timeout)
	}

	// customization: try TCP if UDP fails
	if err != nil || in != nil && in.Truncated {
		tcp := &miekgdns.Client{Net: "tcp", Timeout: q.timeout}
		// If the TCP request succeeds, the error will reset to nil
		var err2 error
		in, _, err2 = tcp.Exchange(m, ns)
		if err2 != nil {
			log.V(1).Error(err2, "DNS (TCP) query failed", "nameserver", ns, "message", m, "timeout", q.timeout)
		} else {
			log.V(1).Info("DNS (TCP) query succeeded", "nameserver", ns, "message", in, "timeout", q.timeout)
		}
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

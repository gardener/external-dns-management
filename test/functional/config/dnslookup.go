/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package config

import (
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
)

const (
	defaultTimeout time.Duration = 5 * time.Second
)

type dnsClient struct {
	server string
	client *dns.Client
}

func createDNSClient(server string) *dnsClient {
	c := &dnsClient{server: server}
	if server == "" {
		return c
	}

	c.client = &dns.Client{ReadTimeout: defaultTimeout}

	return c
}

func (c *dnsClient) LookupHost(name string) ([]string, error) {
	if c.client == nil {
		return net.LookupHost(name)
	}

	response, err := c.lookup(dns.Fqdn(name), dns.TypeA)
	if err != nil {
		return nil, err
	}

	ips := []string{}
	for _, ansa := range response.Answer {
		switch ansb := ansa.(type) {
		case *dns.A:
			ips = append(ips, ansb.A.String())
		}
	}
	return ips, nil
}

func (c *dnsClient) LookupTXT(name string) ([]string, error) {
	if c.client == nil {
		return net.LookupHost(name)
	}

	response, err := c.lookup(dns.Fqdn(name), dns.TypeTXT)
	if err != nil {
		return nil, err
	}

	values := []string{}
	for _, ansa := range response.Answer {
		switch ansb := ansa.(type) {
		case *dns.TXT:
			values = append(values, ansb.Txt...)
		}
	}
	return values, nil
}

func (c *dnsClient) lookup(name string, qtype uint16) (*dns.Msg, error) {
	msg := createMsg(name, qtype)
	r, _, err := c.client.Exchange(msg, c.server+":53")
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("Lookup failed")
	}
	if r.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("Lookup failed with Rcode %v", r.Rcode)
	}
	return r, nil
}

func createMsg(qname string, qtype uint16) *dns.Msg {
	return &dns.Msg{
		MsgHdr: dns.MsgHdr{
			RecursionDesired: true,
			Opcode:           dns.OpcodeQuery,
		},
		Question: []dns.Question{{Name: qname, Qtype: qtype, Qclass: dns.ClassINET}},
	}
}

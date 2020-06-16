/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved.
 * This file is licensed under the Apache Software License, v. 2 except as noted
 * otherwise in the LICENSE file
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

package certmgmt

import (
	"net"
	"strings"
)

var ClusterDomain = "cluster.local"

type CertificateHosts interface {
	GetIPs() []net.IP
	GetDNSNames() []string
}

////////////////////////////////////////////////////////////////////////////////
// No Hosts

type NoHost struct {
}

var _ CertificateHosts = &NoHost{}

func (this *NoHost) GetIPs() []net.IP {
	return nil
}

func (this *NoHost) GetDNSNames() []string {
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// CompoundHosts

type CompoundHosts []CertificateHosts

var _ CertificateHosts = &CompoundHosts{}

func NewCompoundHosts(hosts ...CertificateHosts) CompoundHosts {
	return CompoundHosts(hosts)
}

func (this CompoundHosts) GetDNSNames() []string {
	hosts := []string{}
	for _, h := range this {
		hosts = append(hosts, h.GetDNSNames()...)
	}
	return hosts
}

func (this CompoundHosts) GetIPs() []net.IP {
	hosts := []net.IP{}
	for _, h := range this {
		hosts = append(hosts, h.GetIPs()...)
	}
	return hosts

}

func (this *CompoundHosts) Add(hosts ...CertificateHosts) *CompoundHosts {
	*this = append(*this, hosts...)
	return this
}

func (this CompoundHosts) With(hosts ...CertificateHosts) CompoundHosts {
	return append(this, hosts...)
}

////////////////////////////////////////////////////////////////////////////////
// Plain DNSName

type DNSName struct {
	NoHost
	host string
}

var _ CertificateHosts = &DNSName{}

func NewDNSName(name string) *DNSName {
	return &DNSName{host: name}
}

func (this *DNSName) GetDNSNames() []string {
	return []string{this.host}
}

////////////////////////////////////////////////////////////////////////////////
// Plain IP

type IP struct {
	NoHost
	host net.IP
}

var _ CertificateHosts = &IP{}

func NewIP(name net.IP) *IP {
	return &IP{host: name}
}

func (this *IP) GetIPs() []net.IP {
	return []net.IP{this.host}
}

////////////////////////////////////////////////////////////////////////////////
// Service Hosts

type ServiceHosts struct {
	NoHost
	dns   []string
	hosts []string
}

var _ CertificateHosts = &ServiceHosts{}

func NewServiceHosts(name, namespace string) *ServiceHosts {
	this := &ServiceHosts{
		dns: append([]string{name, namespace, "svc"}, strings.Split(ClusterDomain, ".")...),
	}
	host := ""
	sep := ""
	for _, c := range this.dns {
		host = host + sep + c
		this.hosts = append(this.hosts, host)
		sep = "."
	}
	return this
}

func (this *ServiceHosts) GetDNSNames() []string {
	hosts := make([]string, len(this.hosts))
	copy(hosts, this.hosts)
	return hosts
}

func (this *ServiceHosts) GetName() string {
	return this.dns[0]
}

func (this *ServiceHosts) GetNamespace() string {
	return this.dns[1]
}

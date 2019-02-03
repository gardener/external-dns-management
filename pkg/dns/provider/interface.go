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

package provider

import (
	"context"
	"github.com/gardener/external-dns-management/pkg/dns"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	"k8s.io/apimachinery/pkg/runtime"
)

type Config struct {
	TTL     int64
	Ident   string
	Dryrun  bool
	Factory DNSHandlerFactory
}

func NewConfigForController(c controller.Interface, factory DNSHandlerFactory) Config {
	ident, err := c.GetStringOption(OPT_IDENTIFIER)
	if err != nil {
		ident = "identifier-not-configured"
	}
	ttl, err := c.GetIntOption(OPT_TTL)
	if err != nil {
		ttl = 300
	}
	dryrun, _ := c.GetBoolOption(OPT_DRYRUN)
	return Config{Ident: ident, Dryrun: dryrun, TTL: int64(ttl), Factory: factory}
}

type DNSHostedZone interface {
	Key() string
	Id() string
	Domain() string
	ForwardedDomains() []string
}

type DNSHostedZones []DNSHostedZone

func (this DNSHostedZones) equivalentTo(infos DNSHostedZones) bool {
	if len(this) != len(infos) {
		return false
	}
outer:
	for _, i := range infos {
		for _, t := range this {
			if i.Key() == t.Key() && i.Domain() == t.Domain() {
				continue outer
			}
			return false
		}
	}
	return true
}

type DNSHandlerConfig struct {
	Properties utils.Properties
	Config     *runtime.RawExtension
	DryRun     bool
	Context    context.Context
}

type DNSZoneState interface {
	GetDNSSets() dns.DNSSets
}

type DNSHandler interface {
	GetZones() (DNSHostedZones, error)
	GetZoneState(DNSHostedZone) (DNSZoneState, error)
	ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, reqs []*ChangeRequest) error
}

type DNSHandlerFactory interface {
	TypeCode() string
	Create(logger logger.LogContext, config *DNSHandlerConfig) (DNSHandler, error)
	IsResponsibleFor(object *dnsutils.DNSProviderObject) bool
}

////////////////////////////////////////////////////////////////////////////////

type DNSProviders map[resources.ObjectName]DNSProvider

type DNSProvider interface {
	ObjectName() resources.ObjectName
	Object() resources.Object

	GetZones() DNSHostedZones

	GetZoneState(zone DNSHostedZone) (DNSZoneState, error)
	ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, requests []*ChangeRequest) error

	Match(dns string) int
}

type DoneHandler interface {
	SetInvalid(err error)
	Failed(err error)
	Succeeded()
}


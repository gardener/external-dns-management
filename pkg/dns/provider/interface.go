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
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/flowcontrol"
)

type Config struct {
	TTL               int64
	CacheTTL          time.Duration
	CacheDir          string
	RescheduleDelay   time.Duration
	StatusCheckPeriod time.Duration
	Ident             string
	Dryrun            bool
	ZoneStateCaching  bool
	Delay             time.Duration
	Enabled           utils.StringSet
	Options           *FactoryOptions
	Factory           DNSHandlerFactory
}

func NewConfigForController(c controller.Interface, factory DNSHandlerFactory) (*Config, error) {
	ident, err := c.GetStringOption(OPT_IDENTIFIER)
	if err != nil {
		ident = "identifier-not-configured"
	}
	ttl, err := c.GetIntOption(OPT_TTL)
	if err != nil {
		ttl = 300
	}
	cttl, err := c.GetIntOption(OPT_CACHE_TTL)
	if err != nil {
		cttl = 60
	}
	cdir, _ := c.GetStringOption(OPT_CACHE_DIR)
	dryrun, _ := c.GetBoolOption(OPT_DRYRUN)

	delay, err := c.GetDurationOption(OPT_DNSDELAY)
	if err != nil {
		delay = 10 * time.Second
	}

	rescheduleDelay, err := c.GetDurationOption(OPT_RESCHEDULEDELAY)
	if err != nil {
		rescheduleDelay = 120 * time.Second
	}
	statuscheckperiod, err := c.GetDurationOption(OPT_LOCKSTATUSCHECKPERIOD)
	if err != nil {
		statuscheckperiod = 120 * time.Second
	}

	disableZoneStateCaching, _ := c.GetBoolOption(OPT_DISABLE_ZONE_STATE_CACHING)

	enabled := utils.StringSet{}
	types, err := c.GetStringOption(OPT_PROVIDERTYPES)
	if err != nil || types == "" {
		enabled.AddSet(factory.TypeCodes())
	} else {
		enabled.AddAllSplitted(types)
		if enabled.Contains("all") {
			enabled.Remove("all")
			set := factory.TypeCodes()
			set.Remove("mock-inmemory" /* mock.TYPE_CODE */)
			enabled.AddSet(set)
		}
		_, del := enabled.DiffFrom(factory.TypeCodes())
		if len(del) != 0 {
			return nil, fmt.Errorf("unknown providers types %s", del)
		}
	}

	osrc, _ := c.GetOptionSource(FACTORY_OPTIONS)
	fopts := GetFactoryOptions(osrc)

	return &Config{
		Ident:             ident,
		TTL:               int64(ttl),
		CacheTTL:          time.Duration(cttl) * time.Second,
		CacheDir:          cdir,
		RescheduleDelay:   rescheduleDelay,
		StatusCheckPeriod: statuscheckperiod,
		Dryrun:            dryrun,
		ZoneStateCaching:  !disableZoneStateCaching,
		Delay:             delay,
		Enabled:           enabled,
		Options:           fopts,
		Factory:           factory,
	}, nil
}

type DNSHostedZone interface {
	ProviderType() string
	Key() string
	Id() string
	Domain() string
	ForwardedDomains() []string
	Match(dnsname string) int
	IsPrivate() bool
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
	Logger      logger.LogContext
	Properties  utils.Properties
	Config      *runtime.RawExtension
	DryRun      bool
	Context     context.Context
	CacheConfig ZoneCacheConfig
	Options     *FactoryOptions
	Metrics     Metrics
	RateLimiter flowcontrol.RateLimiter
}

type DNSZoneState interface {
	GetDNSSets() dns.DNSSets
}

const (
	M_LISTZONES  = "list_zones"
	M_PLISTZONES = "list_zones_pages"

	M_LISTRECORDS  = "list_records"
	M_PLISTRECORDS = "list_records_pages"

	M_UPDATERECORDS = "update_records"
	M_PUPDATEREORDS = "update_records_pages"

	M_CREATERECORDS = "create_records"
	M_DELETERECORDS = "delete_records"

	M_CACHED_GETZONES     = "cached_getzones"
	M_CACHED_GETZONESTATE = "cached_getzonestate"
)

type Metrics interface {
	AddGenericRequests(request_type string, n int)
	AddZoneRequests(zoneID, request_type string, n int)
}

type Finalizers interface {
	Finalizers() utils.StringSet
}

type DNSHandler interface {
	ProviderType() string
	GetZones() (DNSHostedZones, error)
	GetZoneState(DNSHostedZone, bool) (DNSZoneState, error)
	ReportZoneStateConflict(zone DNSHostedZone, err error) bool
	ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, reqs []*ChangeRequest) error
	MapTarget(t Target) Target
	Release()
}

type DefaultDNSHandler struct {
	providerType string
}

func NewDefaultDNSHandler(providerType string) DefaultDNSHandler {
	return DefaultDNSHandler{providerType}
}

func (this *DefaultDNSHandler) ProviderType() string {
	return this.providerType
}

func (this *DefaultDNSHandler) MapTarget(t Target) Target {
	return t
}

////////////////////////////////////////////////////////////////////////////////

type DNSHandlerOptionSource interface {
	CreateOptionSource() (local config.OptionSource, defaults *GenericFactoryOptions)
}

type DNSHandlerFactory interface {
	Name() string
	TypeCodes() utils.StringSet
	Create(typecode string, config *DNSHandlerConfig) (DNSHandler, error)
	IsResponsibleFor(object *dnsutils.DNSProviderObject) bool
	SupportZoneStateCache(typecode string) (bool, error)
}

type DNSProviders map[resources.ObjectName]DNSProvider

type DNSProvider interface {
	ObjectName() resources.ObjectName
	Object() resources.Object
	TypeCode() string

	DefaultTTL() int64

	GetZones() DNSHostedZones
	IncludesZone(zoneID string) bool

	GetZoneState(zone DNSHostedZone, forceUpdate bool) (DNSZoneState, error)
	ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, requests []*ChangeRequest) error

	Match(dns string) int
	MatchZone(dns string) int
	IsValid() bool

	AccountHash() string
	MapTarget(t Target) Target

	// ReportZoneStateConflict is used to report a conflict because of stale data.
	// It returns true if zone data will be updated and a retry may resolve the conflict
	ReportZoneStateConflict(zone DNSHostedZone, err error) bool
}

type DoneHandler interface {
	SetInvalid(err error)
	Failed(err error)
	Succeeded()
}

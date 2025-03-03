// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	"github.com/gardener/external-dns-management/pkg/server/remote/embed"
)

type Config struct {
	TTL                                         int64
	CacheTTL                                    time.Duration
	RescheduleDelay                             time.Duration
	StatusCheckPeriod                           time.Duration
	Ident                                       string
	Dryrun                                      bool
	ZoneStateCaching                            bool
	DisableDNSNameValidation                    bool
	Delay                                       time.Duration
	EnabledTypes                                utils.StringSet
	Options                                     *FactoryOptions
	Factory                                     DNSHandlerFactory
	RemoteAccessConfig                          *embed.RemoteAccessServerConfig
	MaxMetadataRecordDeletionsPerReconciliation int
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

	RemoteAccessClientID, err = c.GetStringOption(OPT_REMOTE_ACCESS_CLIENT_ID)
	if err != nil {
		return nil, err
	}

	remoteAccessConfig, err := createRemoteAccessConfig(c)
	if err != nil {
		return nil, err
	}

	disableZoneStateCaching, _ := c.GetBoolOption(OPT_DISABLE_ZONE_STATE_CACHING)
	disableDNSNameValidation, _ := c.GetBoolOption(OPT_DISABLE_DNSNAME_VALIDATION)

	maxMetadataRecordDeletionsPerReconciliation, _ := c.GetIntOption(OPT_MAX_METADATA_RECORD_DELETIONS_PER_RECONCILIATION)

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
		Ident:                    ident,
		TTL:                      int64(ttl),
		CacheTTL:                 time.Duration(cttl) * time.Second,
		RescheduleDelay:          rescheduleDelay,
		StatusCheckPeriod:        statuscheckperiod,
		Dryrun:                   dryrun,
		ZoneStateCaching:         !disableZoneStateCaching,
		DisableDNSNameValidation: disableDNSNameValidation,
		Delay:                    delay,
		EnabledTypes:             enabled,
		Options:                  fopts,
		Factory:                  factory,
		RemoteAccessConfig:       remoteAccessConfig,
		MaxMetadataRecordDeletionsPerReconciliation: maxMetadataRecordDeletionsPerReconciliation,
	}, nil
}

type DNSHostedZone interface {
	Key() string
	Id() dns.ZoneID
	Domain() string
	ForwardedDomains() []string
	Match(dnsname string) int
	IsPrivate() bool
}

type DNSHostedZones []DNSHostedZone

func (this DNSHostedZones) EquivalentTo(infos DNSHostedZones) bool {
	if len(this) != len(infos) {
		return false
	}
outer:
	for _, i := range infos {
		for _, t := range this {
			if i.Key() == t.Key() && i.Domain() == t.Domain() {
				continue outer
			}
		}
		return false
	}
	return true
}

type DNSHandlerConfig struct {
	Logger           logger.LogContext
	Properties       utils.Properties
	Config           *runtime.RawExtension
	DryRun           bool
	Context          context.Context
	ZoneCacheFactory ZoneCacheFactory
	Options          *FactoryOptions
	Metrics          Metrics
	RateLimiter      flowcontrol.RateLimiter
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
	AddGenericRequests(requestType string, n int)
	AddZoneRequests(zoneID, requestType string, n int)
}

type Finalizers interface {
	Finalizers() utils.StringSet
}

type DNSHandler interface {
	ProviderType() string
	GetZones() (DNSHostedZones, error)
	GetZoneState(DNSHostedZone) (DNSZoneState, error)
	ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, reqs []*ChangeRequest) error
	MapTargets(dnsName string, targets []Target) []Target
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

func (this *DefaultDNSHandler) MapTargets(_ string, targets []Target) []Target {
	return targets
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
	IncludesZone(zoneID dns.ZoneID) bool
	HasEquivalentZone(zoneID dns.ZoneID) bool

	GetZoneState(zone DNSHostedZone) (DNSZoneState, error)
	ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, requests []*ChangeRequest) error

	GetDedicatedDNSAccess() DedicatedDNSAccess

	Match(dns string) int
	MatchZone(dns string) int
	IsValid() bool

	AccountHash() string
	MapTargets(dnsName string, targets []Target) []Target
}

type DoneHandler interface {
	SetInvalid(err error)
	Failed(err error)
	Throttled()
	Succeeded()
}

type ProviderEventListener interface {
	ProviderUpdatedEvent(logger logger.LogContext, name resources.ObjectName, annotations map[string]string, handler LightDNSHandler)
	ProviderRemovedEvent(logger logger.LogContext, name resources.ObjectName)
}

type LightDNSHandler interface {
	ProviderType() string
	GetZones() (DNSHostedZones, error)
	GetZoneState(DNSHostedZone) (DNSZoneState, error)
	ExecuteRequests(logger logger.LogContext, zone DNSHostedZone, state DNSZoneState, reqs []*ChangeRequest) error
}

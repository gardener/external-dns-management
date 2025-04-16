// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"reflect"
	"time"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"
	_ "github.com/gardener/controller-manager-library/pkg/resources/defaultscheme/v1.18"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"github.com/gardener/external-dns-management/pkg/apis/dns/crds"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

const CONTROLLER_GROUP_DNS_CONTROLLERS = dns.CONTROLLER_GROUP_DNS_CONTROLLERS

const (
	TARGET_CLUSTER   = source.TARGET_CLUSTER
	PROVIDER_CLUSTER = "provider"
)

const SYNC_ENTRIES = "entries"

const FACTORY_OPTIONS = "factory"

const DNS_POOL = "dns"

var (
	secretGroupKind     = resources.NewGroupKind("", "Secret")
	providerGroupKind   = resources.NewGroupKind(api.GroupName, api.DNSProviderKind)
	entryGroupKind      = resources.NewGroupKind(api.GroupName, api.DNSEntryKind)
	zonePolicyGroupKind = resources.NewGroupKind(api.GroupName, api.DNSHostedZonePolicyKind)
)

// RemoteAccessClientID stores the optional client ID for remote access
var RemoteAccessClientID string

func init() {
	crds.AddToRegistry(apiextensions.DefaultRegistry())
}

func GetFactoryOptions(src config.OptionSource) *FactoryOptions {
	if src == nil {
		return &FactoryOptions{}
	}
	return src.(config.OptionSet).GetSource(FACTORY_OPTIONS).(*FactoryOptions)
}

func CreateFactoryOptionSource(factory DNSHandlerFactory, prefix string) config.OptionSource {
	v := reflect.ValueOf((*FactoryOptions)(nil))
	required := v.Type().Elem().NumField() > 1
	src := &FactoryOptions{GenericFactoryOptions: GenericFactoryOptionDefaults}
	if s, ok := factory.(DNSHandlerOptionSource); ok {
		opts, def := s.CreateOptionSource()
		src.Options = opts
		if def != nil {
			src.GenericFactoryOptions = *def
		}
		required = required || src.Options != nil
	}
	if required {
		// set := &factoryOptionSet{config.NewSharedOptionSet(FACTORY_OPTIONS, prefix, nil)}
		set := config.NewSharedOptionSet(FACTORY_OPTIONS, prefix)
		set.AddSource(FACTORY_OPTIONS, src)
		return set
	}
	return nil
}

func FactoryOptionSourceCreator(factory DNSHandlerFactory) extension.OptionSourceCreator {
	return func() config.OptionSource {
		return CreateFactoryOptionSource(factory, "")
	}
}

func DNSController(name string, factory DNSHandlerFactory) controller.Configuration {
	if name == "" {
		name = factory.Name()
	}
	cfg := controller.Configure(name).
		RequireLease().
		DefaultedStringOption(OPT_CLASS, dns.DEFAULT_CLASS, "Class identifier used to differentiate responsible controllers for entry resources").
		DefaultedBoolOption(OPT_DRYRUN, false, "just check, don't modify").
		DefaultedBoolOption(OPT_DISABLE_ZONE_STATE_CACHING, false, "disable use of cached dns zone state on changes").
		DefaultedBoolOption(OPT_DISABLE_DNSNAME_VALIDATION, false, "disable validation of domain names according to RFC 1123.").
		DefaultedIntOption(OPT_TTL, 300, "Default time-to-live for DNS entries. Defines how long the record is kept in cache by DNS servers or resolvers.").
		DefaultedIntOption(OPT_CACHE_TTL, 120, "Time-to-live for provider hosted zone cache").
		DefaultedIntOption(OPT_SETUP, 10, "number of processors for controller setup").
		DefaultedDurationOption(OPT_DNSDELAY, 10*time.Second, "delay between two dns reconciliations").
		DefaultedDurationOption(OPT_RESCHEDULEDELAY, 120*time.Second, "reschedule delay after losing provider").
		DefaultedDurationOption(OPT_LOCKSTATUSCHECKPERIOD, 120*time.Second, "interval for dns lock status checks").
		DefaultedIntOption(OPT_REMOTE_ACCESS_PORT, 0, "port of remote access server for remote-enabled providers").
		DefaultedStringOption(OPT_REMOTE_ACCESS_CACERT, "", "CA who signed client certs file").
		DefaultedStringOption(OPT_REMOTE_ACCESS_SERVER_SECRET_NAME, "", "name of secret containing remote access server's certificate").
		DefaultedStringOption(OPT_REMOTE_ACCESS_CLIENT_ID, "", "identifier used for remote access").
		DefaultedIntOption(OPT_MAX_METADATA_RECORD_DELETIONS_PER_RECONCILIATION, 50, "maximum number of metadata owner records that can be deleted per zone reconciliation").
		FinalizerDomain("dns.gardener.cloud").
		Reconciler(DNSReconcilerType(factory)).
		Cluster(TARGET_CLUSTER).
		Syncer(SYNC_ENTRIES, controller.NewResourceKey(api.GroupName, api.DNSEntryKind)).
		MainResource(api.GroupName, api.DNSEntryKind).
		DefaultWorkerPool(2, 0).
		Cluster(PROVIDER_CLUSTER).
		CustomResourceDefinitions(providerGroupKind).
		WorkerPool("providers", 2, 10*time.Minute).
		Watches(
			controller.NewResourceKey(api.GroupName, api.DNSProviderKind),
		).
		WorkerPool("secrets", 2, 0).
		Watches(
			controller.NewResourceKey("core", "Secret"),
		).
		MinimalWatches(secretGroupKind).
		WorkerPool("zonepolicies", 1, 0).
		Watches(
			controller.NewResourceKey(api.GroupName, api.DNSHostedZonePolicyKind),
		).
		WorkerPool(DNS_POOL, 1, 15*time.Minute).CommandMatchers(utils.NewStringGlobMatcher(CMD_HOSTEDZONE_PREFIX+"*")).
		OptionSource(FACTORY_OPTIONS, FactoryOptionSourceCreator(factory))
	return cfg
}

type reconciler struct {
	reconcile.DefaultReconciler
	controller controller.Interface
	state      *state
}

var _ reconcile.Interface = &reconciler{}

///////////////////////////////////////////////////////////////////////////////

const KEY_STATE = "dns-state"

func DNSReconcilerType(factory DNSHandlerFactory) controller.ReconcilerType {
	return func(c controller.Interface) (reconcile.Interface, error) {
		return Create(c, factory)
	}
}

///////////////////////////////////////////////////////////////////////////////

func Create(c controller.Interface, factory DNSHandlerFactory) (reconcile.Interface, error) {
	classes := controller.NewClassesByOption(c, OPT_CLASS, source.CLASS_ANNOTATION, dns.DEFAULT_CLASS)
	if f, ok := factory.(Finalizers); ok {
		g := controller.NewFinalizerGroup(c.GetDefinition().FinalizerName(), f.Finalizers())
		c.SetFinalizerHandler(controller.NewFinalizerForGroupAndClasses(g, classes))
	} else {
		c.SetFinalizerHandler(controller.NewFinalizerForClasses(c, c.GetDefinition().FinalizerName(), classes))
	}

	config, err := NewConfigForController(c, factory)
	if err != nil {
		return nil, err
	}

	secretresc, err := c.GetCluster(TARGET_CLUSTER).Resources().GetByGK(secretGroupKind)
	if err != nil {
		return nil, err
	}

	return &reconciler{
		controller: c,
		state: c.GetOrCreateSharedValue(KEY_STATE,
			func() interface{} {
				return NewDNSState(NewDefaultContext(c), secretresc, classes, *config)
			}).(*state),
	}, nil
}

func (this *reconciler) Setup() error {
	this.controller.Infof("*** state Setup ")
	return this.state.Setup()
}

func (this *reconciler) Start() {
	this.state.Start()
}

func (this *reconciler) Command(logger logger.LogContext, cmd string) reconcile.Status {
	zoneid := this.state.DecodeZoneCommand(cmd)
	if zoneid != nil {
		return this.state.ReconcileZone(logger, *zoneid)
	}

	logger.Warnf("got unhandled command %q", cmd)
	return reconcile.Succeeded(logger)
}

func (this *reconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	switch {
	case obj.IsA(&api.DNSProvider{}):
		if this.state.IsResponsibleFor(logger, obj) {
			return this.state.UpdateProvider(logger, dnsutils.DNSProvider(obj))
		} else {
			return this.state.RemoveProvider(logger, dnsutils.DNSProvider(obj))
		}
	case obj.IsA(&api.DNSEntry{}):
		if this.state.IsResponsibleFor(logger, obj) {
			return this.state.UpdateEntry(logger, dnsutils.DNSEntry(obj))
		} else {
			return this.state.EntryDeleted(logger, obj.ClusterKey())
		}
	case obj.IsA(&api.DNSHostedZonePolicy{}):
		if this.state.IsResponsibleFor(logger, obj) {
			return this.state.UpdateZonePolicy(logger, dnsutils.DNSHostedZonePolicy(obj))
		} else {
			return this.state.RemoveZonePolicy(logger, dnsutils.DNSHostedZonePolicy(obj))
		}
	case obj.IsMinimal() && obj.GroupVersionKind().GroupKind() == secretGroupKind:
		return this.state.UpdateSecret(logger, obj)
	}
	return reconcile.Succeeded(logger)
}

func (this *reconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	if this.state.IsResponsibleFor(logger, obj) {
		logger.Debugf("should delete %s", obj.Description())
		switch {
		case obj.IsA(&api.DNSProvider{}):
			return this.state.RemoveProvider(logger, dnsutils.DNSProvider(obj))
		case obj.IsA(&api.DNSEntry{}):
			_ = obj.UpdateFromCache()
			return this.state.DeleteEntry(logger, dnsutils.DNSEntry(obj))
		case obj.IsMinimal() && obj.GroupVersionKind().GroupKind() == secretGroupKind:
			return this.state.UpdateSecret(logger, obj)
		}
	}
	return reconcile.Succeeded(logger)
}

func (this *reconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	logger.Debugf("deleted %s", key)
	switch key.GroupKind() {
	case providerGroupKind:
		return this.state.ProviderDeleted(logger, key.ObjectKey())
	case entryGroupKind:
		return this.state.EntryDeleted(logger, key)
	case zonePolicyGroupKind:
		return this.state.ZonePolicyDeleted(logger, key)
	}
	return reconcile.Succeeded(logger)
}

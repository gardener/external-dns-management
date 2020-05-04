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
	"reflect"
	"time"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"

	"github.com/gardener/external-dns-management/pkg/apis/dns/crds"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/source"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	corev1 "k8s.io/api/core/v1"

	_ "github.com/gardener/controller-manager-library/pkg/resources/defaultscheme/v1.12"
)

const CONTROLLER_GROUP_DNS_CONTROLLERS = dns.CONTROLLER_GROUP_DNS_CONTROLLERS

const TARGET_CLUSTER = source.TARGET_CLUSTER
const PROVIDER_CLUSTER = "provider"

const SYNC_ENTRIES = "entries"

const FACTORY_OPTIONS = "factory"

var ownerGroupKind = resources.NewGroupKind(api.GroupName, api.DNSOwnerKind)
var providerGroupKind = resources.NewGroupKind(api.GroupName, api.DNSProviderKind)
var entryGroupKind = resources.NewGroupKind(api.GroupName, api.DNSEntryKind)

func init() {
	crds.AddToRegistry(apiextensions.DefaultRegistry())
}

func GetFactoryOptions(src config.OptionSource) *FactoryOptions {
	if src == nil {
		return &FactoryOptions{}
	}
	return src.(config.OptionSet).GetSource(FACTORY_OPTIONS).(*FactoryOptions)
}

type factoryOptionSet struct {
	*config.SharedOptionSet
}

func (this *factoryOptionSet) AddOptionsToSet(set config.OptionSet) {
	this.SharedOptionSet.AddOptionsToSet(set)
}

func (this *factoryOptionSet) Evaluate() error {
	return this.SharedOptionSet.Evaluate()
}

func CreateFactoryOptionSource(factory DNSHandlerFactory, prefix string) config.OptionSource {
	v := reflect.ValueOf((*FactoryOptions)(nil))
	required := v.Type().Elem().NumField() > 1
	src := &FactoryOptions{}
	if s, ok := factory.(DNSHandlerOptionSource); ok {
		src.Options = s.CreateOptionSource()
		required = required || src.Options != nil
	}
	if required {
		//set := &factoryOptionSet{config.NewSharedOptionSet(FACTORY_OPTIONS, prefix, nil)}
		set := config.NewSharedOptionSet(FACTORY_OPTIONS, prefix, nil)
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
		DefaultedStringOption(OPT_IDENTIFIER, "dnscontroller", "Identifier used to mark DNS entries in DNS system").
		DefaultedStringOption(OPT_CACHE_DIR, "", "Directory to store zone caches (for reload after restart)").
		DefaultedBoolOption(OPT_DRYRUN, false, "just check, don't modify").
		DefaultedBoolOption(OPT_DISABLE_ZONE_STATE_CACHING, false, "disable use of cached dns zone state on changes").
		DefaultedIntOption(OPT_TTL, 300, "Default time-to-live for DNS entries").
		DefaultedIntOption(OPT_CACHE_TTL, 120, "Time-to-live for provider hosted zone cache").
		DefaultedIntOption(OPT_SETUP, 10, "number of processors for controller setup").
		DefaultedDurationOption(OPT_DNSDELAY, 10*time.Second, "delay between two dns reconciliations").
		DefaultedDurationOption(OPT_RESCHEDULEDELAY, 120*time.Second, "reschedule delay after losing provider").
		FinalizerDomain("dns.gardener.cloud").
		Reconciler(DNSReconcilerType(factory)).
		Cluster(TARGET_CLUSTER).
		Syncer(SYNC_ENTRIES, controller.NewResourceKey(api.GroupName, api.DNSEntryKind)).
		CustomResourceDefinitions(ownerGroupKind, entryGroupKind).
		MainResource(api.GroupName, api.DNSEntryKind).
		DefaultWorkerPool(2, 0).
		WorkerPool("ownerids", 1, 0).
		Watches(
			controller.NewResourceKey(api.GroupName, api.DNSOwnerKind),
		).
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
		WorkerPool("dns", 1, 15*time.Minute).CommandMatchers(utils.NewStringGlobMatcher(CMD_HOSTEDZONE_PREFIX+"*")).
		WorkerPool("statistic", 1, 0).Commands(CMD_STATISTIC).
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

	zoneCacheCleanupOutdated(c, config.CacheDir, ZoneCachePrefix)

	ownerresc, err := c.GetCluster(TARGET_CLUSTER).Resources().GetByGK(ownerGroupKind)
	if err != nil {
		return nil, err
	}

	return &reconciler{
		controller: c,
		state: c.GetOrCreateSharedValue(KEY_STATE,
			func() interface{} {
				return NewDNSState(NewDefaultContext(c), ownerresc, classes, *config)
			}).(*state),
	}, nil
}

func (this *reconciler) Setup() {
	this.controller.Infof("*** state Setup ")
	this.state.Setup()
}

func (this *reconciler) Start() {
	this.controller.GetPool("dns").StartTicker()
	this.state.Start()
}

func (this *reconciler) Command(logger logger.LogContext, cmd string) reconcile.Status {
	if cmd == CMD_STATISTIC {
		this.state.UpdateOwnerCounts(logger)
	} else {
		zoneid := this.state.DecodeZoneCommand(cmd)
		if zoneid != "" {
			return this.state.ReconcileZone(logger, zoneid)
		}
		logger.Infof("got unhandled command %q", cmd)
	}
	return reconcile.Succeeded(logger)
}

func (this *reconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	switch {
	case obj.IsA(&api.DNSOwner{}):
		if this.state.IsResponsibleFor(logger, obj) {
			return this.state.UpdateOwner(logger, dnsutils.DNSOwner(obj), false)
		} else {
			return this.state.OwnerDeleted(logger, obj.Key())
		}
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
	case obj.IsA(&corev1.Secret{}):
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
			obj.UpdateFromCache()
			return this.state.DeleteEntry(logger, dnsutils.DNSEntry(obj))
		case obj.IsA(&corev1.Secret{}):
			return this.state.UpdateSecret(logger, obj)
		}
	}
	return reconcile.Succeeded(logger)
}

func (this *reconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	logger.Debugf("deleted %s", key)
	switch key.GroupKind() {
	case ownerGroupKind:
		return this.state.OwnerDeleted(logger, key.ObjectKey())
	case providerGroupKind:
		return this.state.ProviderDeleted(logger, key.ObjectKey())
	case entryGroupKind:
		return this.state.EntryDeleted(logger, key)
	}
	return reconcile.Succeeded(logger)
}

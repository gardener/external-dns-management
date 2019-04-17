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
	"time"

	"github.com/gardener/external-dns-management/pkg/crds"
	"github.com/gardener/external-dns-management/pkg/dns/source"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"

	corev1 "k8s.io/api/core/v1"
)

const CONTROLLER_GROUP_DNS_CONTROLLERS = "dnscontrollers"

const TARGET_CLUSTER = source.TARGET_CLUSTER
const PROVIDER_CLUSTER = "provider"

var ownerGroupKind = resources.NewGroupKind(api.GroupName, api.DNSOwnerKind)
var providerGroupKind = resources.NewGroupKind(api.GroupName, api.DNSProviderKind)
var entryGroupKind = resources.NewGroupKind(api.GroupName, api.DNSEntryKind)

func DNSController(name string, factory DNSHandlerFactory) controller.Configuration {
	return controller.Configure(name).
		RequireLease().
		DefaultedStringOption(OPT_CLASS, dnsutils.DEFAULT_CLASS, "Identifier used to differentiate responsible controllers for entries").
		DefaultedStringOption(OPT_IDENTIFIER, "dnscontroller", "Identifier used to mark DNS entries").
		DefaultedBoolOption(OPT_DRYRUN, false, "just check, don't modify").
		DefaultedIntOption(OPT_TTL, 300, "Default time-to-live for DNS entries").
		DefaultedIntOption(OPT_CACHE_TTL, 120, "Time-to-live for provider hosted zone cache").
		DefaultedIntOption(OPT_SETUP, 10, "number of processors for controller setup").
		DefaultedDurationOption(OPT_DNSDELAY, 10*time.Second, "delay between two dns reconcilations").
		Reconciler(DNSReconcilerType(factory)).
		Cluster(TARGET_CLUSTER).
		CustomResourceDefinitions(crds.DNSEntryCRD, crds.DNSOwnerCRD).
		MainResource(api.GroupName, api.DNSEntryKind).
		DefaultWorkerPool(2, 0).
		WorkerPool("ownerids", 1, 0).
		Watches(
			controller.NewResourceKey(api.GroupName, api.DNSOwnerKind),
		).
		Cluster(PROVIDER_CLUSTER).
		CustomResourceDefinitions(crds.DNSProviderCRD).
		WorkerPool("providers", 2, 10*time.Minute).
		Watches(
			controller.NewResourceKey(api.GroupName, api.DNSProviderKind),
		).
		WorkerPool("secrets", 2, 0).
		Watches(
			controller.NewResourceKey("core", "Secret"),
		).
		WorkerPool("dns", 1, 15*time.Minute).CommandMatchers(utils.NewStringGlobMatcher(HOSTEDZONE_PREFIX + "*"))
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
	copt, _ := c.GetStringOption(OPT_CLASS)
	classes := dnsutils.NewClasses(copt)
	c.SetFinalizerHandler(dnsutils.NewFinalizer(c, c.GetDefinition().FinalizerName(), classes))
	return &reconciler{
		controller: c,
		state: c.GetOrCreateSharedValue(KEY_STATE,
			func() interface{} {
				return NewDNSState(c, classes, NewConfigForController(c, factory))
			}).(*state),
	}, nil
}

func (this *reconciler) Setup() {
	this.controller.Infof("*** State Setup ")
	this.state.Setup()
}

func (this *reconciler) Start() {
	this.controller.GetPool("dns").StartTicker()
	this.state.Start()
}

func (this *reconciler) Command(logger logger.LogContext, cmd string) reconcile.Status {

	zoneid := this.state.DecodeZoneCommand(cmd)
	if zoneid != "" {
		return this.state.ReconcileZone(logger, zoneid)
	}
	logger.Infof("got unhandled command %q", cmd)
	return reconcile.Succeeded(logger)
}

func (this *reconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	switch {
	case obj.IsA(&api.DNSOwner{}):
		if this.state.IsResponsibleFor(logger, obj) {
			return this.state.UpdateOwner(logger, dnsutils.DNSOwner(obj))
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
			return this.state.EntryDeleted(logger, obj.Key())
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
		return this.state.EntryDeleted(logger, key.ObjectKey())
	}
	return reconcile.Succeeded(logger)
}

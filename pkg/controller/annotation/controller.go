/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package annotation

import (
	"time"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"

	"github.com/gardener/external-dns-management/pkg/apis/dns/crds"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/controller/annotation/annotations"
	"github.com/gardener/external-dns-management/pkg/dns"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
)

const CONTROLLER = "annotation"

func init() {
	crds.AddToRegistry(apiextensions.DefaultRegistry())

	controller.Configure(CONTROLLER).
		Reconciler(Create).
		DefaultWorkerPool(5, 0*time.Second).
		OptionsByExample("options", &Config{}).
		CustomResourceDefinitions(resources.NewGroupKind(api.GroupName, api.DNSAnnotationKind)).
		MainResource(api.GroupName, api.DNSAnnotationKind).
		ActivateExplicitly().
		MustRegister()
}

type Config struct {
	processors int
}

func (this *Config) AddOptionsToSet(set config.OptionSet) {
	set.AddIntOption(&this.processors, dns.OPT_SETUP, "", 10, "number of processors for controller setup")
}

func (this *Config) Evaluate() error {
	return nil
}

type reconciler struct {
	reconcile.DefaultReconciler
	controller  controller.Interface
	config      *Config
	annotations *annotations.State
}

var _ reconcile.Interface = &reconciler{}

///////////////////////////////////////////////////////////////////////////////

func Create(controller controller.Interface) (reconcile.Interface, error) {

	cfg, err := controller.GetOptionSource("options")
	config := cfg.(*Config)
	if err == nil {
		controller.Infof("using %d processors for setups", config.processors)
	}

	return &reconciler{
		controller:  controller,
		config:      config,
		annotations: annotations.GetOrCreateWatches(controller),
	}, nil
}

func (this *reconciler) Setup() {
	this.controller.Infof("### setup dns watch resources")
	res, _ := this.controller.GetMainCluster().Resources().GetByExample(&api.DNSAnnotation{})
	list, _ := res.ListCached(labels.Everything())
	dnsutils.ProcessElements(list, func(e resources.Object) {
		this.annotations.Add(this.controller, e)
	}, this.config.processors)
}

///////////////////////////////////////////////////////////////////////////////

func (this *reconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	err := this.annotations.Add(logger, obj)
	return reconcile.FailedOnError(logger, err)
}

func (this *reconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	this.annotations.Remove(logger, obj.ClusterKey())
	return reconcile.Succeeded(logger)
}

func (this *reconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	this.annotations.Remove(logger, key)
	return reconcile.Succeeded(logger)
}

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package annotation

import (
	"time"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"
	"k8s.io/apimachinery/pkg/labels"

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

func (this *reconciler) Setup() error {
	this.controller.Infof("### setup dns watch resources")
	res, _ := this.controller.GetMainCluster().Resources().GetByExample(&api.DNSAnnotation{})
	list, _ := res.ListCached(labels.Everything())
	return dnsutils.ProcessElements(list, func(e resources.Object) error {
		return this.annotations.Add(this.controller, e)
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

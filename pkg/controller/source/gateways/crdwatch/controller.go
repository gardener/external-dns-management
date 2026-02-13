// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crdwatch

import (
	"fmt"
	"os"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/external-dns-management/pkg/controller/source/gateways/gatewayapi"
	"github.com/gardener/external-dns-management/pkg/controller/source/gateways/istio"
	"github.com/gardener/external-dns-management/pkg/dns/source"
)

const CONTROLLER = "watch-gateways-crds"

func init() {
	controller.Configure(CONTROLLER).
		Reconciler(Create).
		DefaultWorkerPool(1, 0*time.Second).
		MainResource(apiextensionsv1.GroupName, "CustomResourceDefinition").
		MustRegister(source.CONTROLLER_GROUP_DNS_SOURCES)
}

type reconciler struct {
	reconcile.DefaultReconciler
	controller controller.Interface

	currentActivated sets.Set[string]
}

var _ reconcile.Interface = &reconciler{}

///////////////////////////////////////////////////////////////////////////////

func Create(controller controller.Interface) (reconcile.Interface, error) {
	return &reconciler{
		controller: controller,
	}, nil
}

func (r *reconciler) Setup() error {
	r.controller.Infof("### setup crds watch resources")
	toBeActivated, err := r.checkRelevantCRDs()
	if err != nil {
		return fmt.Errorf("could not check for relevant CRDs: %w", err)
	}

	if gatewayapi.Deactivated && toBeActivated.Has("gateway.networking.k8s.io") {
		r.controller.Info("### k8s gateway relevant CRDs found but gatewayapi source controller deactivated: need to restart to initialise controller")
		os.Exit(3)
	}

	if istio.Deactivated && toBeActivated.Has("networking.istio.io") {
		r.controller.Info("### istio relevant CRDs found but istio source controller deactivated: need to restart to initialise controller")
		os.Exit(3)
	}

	r.currentActivated = toBeActivated

	return nil
}

///////////////////////////////////////////////////////////////////////////////

func (r *reconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	return r.checkRelevantCRDChange(logger, obj)
}

func (r *reconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	return r.checkRelevantCRDChange(logger, obj)
}

func (r *reconciler) checkRelevantCRDChange(logger logger.LogContext, obj resources.Object) reconcile.Status {
	crd := obj.Data().(*apiextensionsv1.CustomResourceDefinition)
	if crd.Spec.Group == "gateway.networking.k8s.io" || crd.Spec.Group == "networking.istio.io" {
		toBeActivated, err := r.checkRelevantCRDs()
		if err != nil {
			return reconcile.Failed(logger, fmt.Errorf("could not check for relevant CRDs: %w", err))
		}
		if !toBeActivated.Equal(r.currentActivated) {
			logger.Infof("activated groups changed from %v to %v: need to restart to update controllers", r.currentActivated, toBeActivated)
			os.Exit(3)
		}
	}
	return reconcile.Succeeded(logger)
}

func (r *reconciler) Deleted(logger logger.LogContext, _ resources.ClusterObjectKey) reconcile.Status {
	return reconcile.Succeeded(logger)
}

func (r *reconciler) checkRelevantCRDs() (sets.Set[string], error) {
	relevantCRDsGatewayapi := sets.New[string]("gateways.gateway.networking.k8s.io", "httproutes.gateway.networking.k8s.io")
	relevantCRDsIstio := sets.New[string]("gateways.networking.istio.io", "virtualservices.networking.istio.io")
	res, err := r.controller.GetMainCluster().Resources().GetByExample(&apiextensionsv1.CustomResourceDefinition{})
	if err != nil {
		return nil, err
	}
	list, err := res.ListCached(labels.Everything())
	if err != nil {
		return nil, err
	}
	countGatewayapiCRDs := 0
	countIstioCRDs := 0
	for _, item := range list {
		crd := item.Data().(*apiextensionsv1.CustomResourceDefinition)
		name := crdName(crd)
		if relevantCRDsGatewayapi.Has(name) {
			countGatewayapiCRDs++
		}
		if relevantCRDsIstio.Has(name) {
			countIstioCRDs++
		}
	}

	activatedGroups := sets.New[string]()
	if countGatewayapiCRDs == 2 {
		activatedGroups.Insert("gateway.networking.k8s.io")
	}
	if countIstioCRDs == 2 {
		activatedGroups.Insert("networking.istio.io")
	}
	return activatedGroups, nil
}

func crdName(crd *apiextensionsv1.CustomResourceDefinition) string {
	return crd.Spec.Names.Plural + "." + crd.Spec.Group
}

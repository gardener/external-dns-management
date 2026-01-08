// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crdwatch

import (
	"os"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/gardener/external-dns-management/pkg/controller/source/gateways/gatewayapi"
	"github.com/gardener/external-dns-management/pkg/controller/source/gateways/istio"
	"github.com/gardener/external-dns-management/pkg/dns/source"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
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

	relevantCustomResourceDefinitionDeployed map[string]bool
}

var _ reconcile.Interface = &reconciler{}

///////////////////////////////////////////////////////////////////////////////

func Create(controller controller.Interface) (reconcile.Interface, error) {
	return &reconciler{
		controller: controller,
		relevantCustomResourceDefinitionDeployed: map[string]bool{
			"gateways.networking.istio.io":         false,
			"virtualservices.networking.istio.io":  false,
			"gateways.gateway.networking.k8s.io":   false,
			"httproutes.gateway.networking.k8s.io": false,
		},
	}, nil
}

func (r *reconciler) Setup() error {
	r.controller.Infof("### setup crds watch resources")
	res, _ := r.controller.GetMainCluster().Resources().GetByExample(&apiextensionsv1.CustomResourceDefinition{})
	list, _ := res.ListCached(labels.Everything())
	return dnsutils.ProcessElements(list, func(e resources.Object) error {
		crd := e.Data().(*apiextensionsv1.CustomResourceDefinition)
		switch crd.Spec.Group {
		case "networking.istio.io", "gateway.networking.k8s.io":
			name := crdName(crd)
			if _, relevant := r.relevantCustomResourceDefinitionDeployed[name]; relevant {
				r.relevantCustomResourceDefinitionDeployed[name] = true
				switch crd.Spec.Group {
				case "networking.istio.io":
					if istio.Deactivated {
						r.controller.Infof("### istio relevant CRD %s found but istio source controller deactivated: need to restart to initialise controller", name)
						os.Exit(3)
					}
				case "gateway.networking.k8s.io":
					if gatewayapi.Deactivated {
						r.controller.Infof("### k8s gateway relevant CRD %s found but gatewayapi source controller deactivated: need to restart to initialise controller", name)
						os.Exit(3)
					}
				}
			}
			return nil
		default:
			return nil
		}
	}, 1)
}

///////////////////////////////////////////////////////////////////////////////

func (r *reconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	crd := obj.Data().(*apiextensionsv1.CustomResourceDefinition)
	name := crdName(crd)
	if alreadyDeployed, relevant := r.relevantCustomResourceDefinitionDeployed[name]; relevant && !alreadyDeployed {
		logger.Infof("new relevant CRD %s deployed: need to restart to initialise controller", name)
		os.Exit(2)
	}
	return reconcile.Succeeded(logger)
}

func (r *reconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	crd := obj.Data().(*apiextensionsv1.CustomResourceDefinition)
	name := crdName(crd)
	if alreadyDeployed, relevant := r.relevantCustomResourceDefinitionDeployed[name]; relevant && alreadyDeployed {
		logger.Infof("new relevant CRD %s deleted: need to restart to disable controllers", name)
		os.Exit(3)
	}
	return reconcile.Succeeded(logger)
}

func (r *reconciler) Deleted(logger logger.LogContext, _ resources.ClusterObjectKey) reconcile.Status {
	return reconcile.Succeeded(logger)
}

func crdName(crd *apiextensionsv1.CustomResourceDefinition) string {
	return crd.Spec.Names.Plural + "." + crd.Spec.Group
}

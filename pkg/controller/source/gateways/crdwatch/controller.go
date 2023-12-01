/*
 * Copyright 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package crdwatch

import (
	"os"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/external-dns-management/pkg/dns/source"
	dnsutils "github.com/gardener/external-dns-management/pkg/dns/utils"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/labels"
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

func (r *reconciler) Setup() {
	r.controller.Infof("### setup crds watch resources")
	res, _ := r.controller.GetMainCluster().Resources().GetByExample(&apiextensionsv1.CustomResourceDefinition{})
	list, _ := res.ListCached(labels.Everything())
	dnsutils.ProcessElements(list, func(e resources.Object) error {
		crd := e.Data().(*apiextensionsv1.CustomResourceDefinition)
		switch crd.Spec.Group {
		case "networking.istio.io", "gateway.networking.k8s.io":
			name := crdName(crd)
			if _, relevant := r.relevantCustomResourceDefinitionDeployed[name]; relevant {
				r.relevantCustomResourceDefinitionDeployed[name] = true
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

func (r *reconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	return reconcile.Succeeded(logger)
}

func crdName(crd *apiextensionsv1.CustomResourceDefinition) string {
	return crd.Spec.Names.Plural + "." + crd.Spec.Group
}

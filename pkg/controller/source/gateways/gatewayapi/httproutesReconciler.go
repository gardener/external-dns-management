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

package gatewayapi

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func HTTPRoutesReconciler(c controller.Interface) (reconcile.Interface, error) {
	state, err := getOrCreateSharedState(c)
	if err != nil {
		return nil, err
	}
	return &httproutesReconciler{controller: c, state: state}, nil
}

var _ reconcile.Interface = &httproutesReconciler{}

type httproutesReconciler struct {
	reconcile.DefaultReconciler
	controller controller.Interface
	state      *routesState
}

func (r *httproutesReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	gateways := extractGatewayNames(obj.Data())
	oldGateways := r.state.AddRoute(obj.ObjectName(), gateways)
	gateways.Add(oldGateways...)
	r.triggerGateways(obj.ClusterKey().Cluster(), gateways)
	return reconcile.Succeeded(logger)
}

func (r *httproutesReconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	r.state.RemoveRoute(obj.ObjectName())
	r.triggerGateways(obj.ClusterKey().Cluster(), extractGatewayNames(obj.Data()))
	return reconcile.Succeeded(logger)
}

func (r *httproutesReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	gateways := r.state.MatchingGatewaysByRoute(key.ObjectName())
	r.state.RemoveRoute(key.ObjectName())
	r.triggerGateways(key.Cluster(), resources.NewObjectNameSetByArray(gateways))
	return reconcile.Succeeded(logger)
}

func (r *httproutesReconciler) triggerGateways(cluster string, gateways resources.ObjectNameSet) {
	for g := range gateways {
		_ = r.controller.EnqueueKey(resources.NewClusterKeyForObject(cluster, g.ForGroupKind(GroupKindGateway)))
	}
}

func extractGatewayNames(route resources.ObjectData) resources.ObjectNameSet {
	gatewayNames := resources.NewObjectNameSet()
	switch data := route.(type) {
	case *gatewayapisv1.HTTPRoute:
		for _, ref := range data.Spec.ParentRefs {
			if (ref.Group == nil || string(*ref.Group) == GroupKindGateway.Group) &&
				(ref.Kind == nil || string(*ref.Kind) == GroupKindGateway.Kind) {
				namespace := data.Namespace
				if ref.Namespace != nil {
					namespace = string(*ref.Namespace)
				}
				gatewayNames.Add(resources.NewObjectName(namespace, string(ref.Name)))
			}
		}
	case *gatewayapisv1alpha2.HTTPRoute:
		for _, ref := range data.Spec.ParentRefs {
			if (ref.Group == nil || string(*ref.Group) == GroupKindGateway.Group) &&
				(ref.Kind == nil || string(*ref.Kind) == GroupKindGateway.Kind) {
				namespace := data.Namespace
				if ref.Namespace != nil {
					namespace = string(*ref.Namespace)
				}
				gatewayNames.Add(resources.NewObjectName(namespace, string(ref.Name)))
			}
		}
	}
	return gatewayNames
}

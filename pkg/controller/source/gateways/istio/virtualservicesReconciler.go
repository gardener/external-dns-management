// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package istio

import (
	"strings"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	istionetworkingv1alpha3 "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istionetworkingv1beta1 "istio.io/client-go/pkg/apis/networking/v1beta1"
)

func newVirtualServicesReconciler(c controller.Interface) (reconcile.Interface, error) {
	state, err := getOrCreateSharedState(c)
	if err != nil {
		return nil, err
	}
	return &virtualservicesReconciler{controller: c, state: state}, nil
}

var _ reconcile.Interface = &virtualservicesReconciler{}

type virtualservicesReconciler struct {
	reconcile.DefaultReconciler
	controller controller.Interface
	state      *resourcesState
}

func (r *virtualservicesReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	gateways := extractGatewayNames(obj.Data())
	oldGateways := r.state.AddVirtualService(obj.ObjectName(), gateways)
	gateways.Add(oldGateways...)
	r.triggerGateways(obj.ClusterKey().Cluster(), gateways)
	return reconcile.Succeeded(logger)
}

func (r *virtualservicesReconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	r.state.RemoveVirtualService(obj.ObjectName())
	r.triggerGateways(obj.ClusterKey().Cluster(), extractGatewayNames(obj.Data()))
	return reconcile.Succeeded(logger)
}

func (r *virtualservicesReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	gateways := r.state.MatchingGatewaysByVirtualService(key.ObjectName())
	r.state.RemoveVirtualService(key.ObjectName())
	r.triggerGateways(key.Cluster(), resources.NewObjectNameSetByArray(gateways))
	return reconcile.Succeeded(logger)
}

func (r *virtualservicesReconciler) triggerGateways(cluster string, gateways resources.ObjectNameSet) {
	for g := range gateways {
		_ = r.controller.EnqueueKey(resources.NewClusterKeyForObject(cluster, g.ForGroupKind(GroupKindGateway)))
	}
}

func extractGatewayNames(virtualService resources.ObjectData) resources.ObjectNameSet {
	gatewayNames := resources.NewObjectNameSet()
	switch data := virtualService.(type) {
	case *istionetworkingv1beta1.VirtualService:
		for _, name := range data.Spec.Gateways {
			if objName := toObjectName(name, virtualService.GetNamespace()); objName != nil {
				gatewayNames.Add(*objName)
			}
		}
	case *istionetworkingv1alpha3.VirtualService:
		for _, name := range data.Spec.Gateways {
			if objName := toObjectName(name, virtualService.GetNamespace()); objName != nil {
				gatewayNames.Add(*objName)
			}
		}
	}
	return gatewayNames
}

func toObjectName(name, defaultNamespace string) *resources.ObjectName {
	parts := strings.Split(name, "/")
	switch len(parts) {
	case 1:
		var objName resources.ObjectName = resources.NewObjectName(defaultNamespace, name)
		return &objName
	case 2:
		var objName resources.ObjectName = resources.NewObjectName(parts...)
		return &objName
	}
	return nil
}

// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package gatewayapi

import (
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gatewayapisv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapisv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// namespaceLister fetches the labels of a namespace, needed for the
// allowedRoutes "Selector" case.
type namespaceLister interface {
	NamespaceLabels(name string) (labels.Set, error)
}

var _ namespaceLister = &cachedNamespaceLister{}

type cachedNamespaceLister struct {
	namespaces resources.Interface
}

func newNamespaceLister(c controller.Interface) (*cachedNamespaceLister, error) {
	nsResources, err := c.GetMainCluster().Resources().GetByExample(&corev1.Namespace{})
	if err != nil {
		return nil, err
	}
	return &cachedNamespaceLister{namespaces: nsResources}, nil
}

func (l *cachedNamespaceLister) NamespaceLabels(name string) (labels.Set, error) {
	obj, err := l.namespaces.GetCached(name)
	if err != nil {
		return nil, err
	}
	return obj.GetLabels(), nil
}

// gatewayAdmitsNamespace reports whether the gateway's listeners permit routes
// from routeNamespace to attach, per Gateway API allowedRoutes semantics. A
// route is admitted if at least one listener admits it. The default (unset
// allowedRoutes) is "Same", i.e. only the gateway's own namespace.
func gatewayAdmitsNamespace(nsLister namespaceLister, gatewayNamespace, routeNamespace string, allowedRoutes []*gatewayapisv1.AllowedRoutes) (bool, error) {
	if routeNamespace == gatewayNamespace {
		return true, nil
	}
	for _, ar := range allowedRoutes {
		from := gatewayapisv1.NamespacesFromSame
		if ar != nil && ar.Namespaces != nil && ar.Namespaces.From != nil {
			from = *ar.Namespaces.From
		}
		switch from {
		case gatewayapisv1.NamespacesFromAll:
			return true, nil
		case gatewayapisv1.NamespacesFromSelector:
			if ar.Namespaces.Selector == nil {
				continue
			}
			selector, err := metav1.LabelSelectorAsSelector(ar.Namespaces.Selector)
			if err != nil {
				return false, err
			}
			nsLabels, err := nsLister.NamespaceLabels(routeNamespace)
			if err != nil {
				return false, err
			}
			if selector.Matches(nsLabels) {
				return true, nil
			}
		}
	}
	return false, nil
}

// listenerAllowedRoutes extracts the per-listener allowedRoutes of a gateway,
// normalizing both API versions to the v1 type.
func listenerAllowedRoutes(obj resources.ObjectData) []*gatewayapisv1.AllowedRoutes {
	var result []*gatewayapisv1.AllowedRoutes
	switch data := obj.(type) {
	case *gatewayapisv1.Gateway:
		for _, l := range data.Spec.Listeners {
			result = append(result, l.AllowedRoutes)
		}
	case *gatewayapisv1beta1.Gateway:
		for _, l := range data.Spec.Listeners {
			result = append(result, l.AllowedRoutes) // v1beta1.AllowedRoutes is an alias for v1.AllowedRoutes
		}
	}
	return result
}

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gateways_crd_watchdog

import (
	"context"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/gardener/cert-management/pkg/certman2/controller/source/istio_gateway"
	"github.com/gardener/cert-management/pkg/certman2/controller/source/k8s_gateway"
)

// ControllerName is the name of this controller.
const ControllerName = "gateways-crd-watchdog"

const (
	istioGatewaysCRD        = "gateways.networking.istio.io"
	istioVirtualServicesCRD = "virtualservices.networking.istio.io"
	k8sGatewaysCRD          = "gateways.gateway.networking.k8s.io"
	k8sHTTPRoutesCRD        = "httproutes.gateway.networking.k8s.io"
)

var relevantCRDs = []string{
	istioGatewaysCRD,
	istioVirtualServicesCRD,
	k8sGatewaysCRD,
	k8sHTTPRoutesCRD,
}

// CheckGatewayCRDsState contains the state of the gateway CRD check.
type CheckGatewayCRDsState struct {
	relevantCRDDeployed      map[string]string
	istioGatewayVersion      istio_gateway.Version
	kubernetesGatewayVersion k8s_gateway.Version
}

// CheckGatewayCRDs checks for relevant gateway custom resource definition deployments.
func CheckGatewayCRDs(ctx context.Context, c client.Client) (*CheckGatewayCRDsState, error) {
	state := CheckGatewayCRDsState{
		relevantCRDDeployed: map[string]string{},
	}
	for _, name := range relevantCRDs {
		state.relevantCRDDeployed[name] = ""
	}

	list := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := c.List(ctx, list); err != nil {
		return nil, fmt.Errorf("listing custom resource definitions failed: %w", err)
	}
	for _, crd := range list.Items {
		name := crd.GetName()
		switch name {
		case istioGatewaysCRD:
			v := istio_gateway.GetPreferredVersion(&crd)
			state.istioGatewayVersion = v
			state.relevantCRDDeployed[name] = string(v)
		case istioVirtualServicesCRD:
			state.relevantCRDDeployed[name] = string(istio_gateway.GetPreferredVersion(&crd))
		case k8sGatewaysCRD:
			v := k8s_gateway.GetPreferredVersion(&crd)
			state.kubernetesGatewayVersion = v
			state.relevantCRDDeployed[name] = string(v)
		case k8sHTTPRoutesCRD:
			state.relevantCRDDeployed[name] = string(k8s_gateway.GetPreferredVersion(&crd))
		}
	}
	return &state, nil
}

// IstioGatewayVersion returns istio gateway version to watch.
func (s *CheckGatewayCRDsState) IstioGatewayVersion() (istio_gateway.Version, error) {
	for _, name := range []string{istioGatewaysCRD, istioVirtualServicesCRD} {
		if s.relevantCRDDeployed[name] == "" {
			return istio_gateway.VersionNone, fmt.Errorf("no crd %s found", name)
		}
		if s.relevantCRDDeployed[name] != string(s.istioGatewayVersion) {
			return istio_gateway.VersionNone, fmt.Errorf("inconsistent crd %s: version mismatch %q != %q found", name, s.relevantCRDDeployed[name], string(s.istioGatewayVersion))
		}
	}
	return s.istioGatewayVersion, nil
}

// KubernetesGatewayVersion returns Kubernetes Gateway API gateway version to watch.
func (s *CheckGatewayCRDsState) KubernetesGatewayVersion() (k8s_gateway.Version, error) {
	for _, name := range []string{k8sGatewaysCRD, k8sHTTPRoutesCRD} {
		if s.relevantCRDDeployed[name] == "" {
			return k8s_gateway.VersionNone, fmt.Errorf("no crd %s found", name)
		}
		if s.relevantCRDDeployed[name] != string(s.kubernetesGatewayVersion) {
			return k8s_gateway.VersionNone, fmt.Errorf("inconsistent crd %s: version mismatch %q != %q found", name, s.relevantCRDDeployed[name], string(s.kubernetesGatewayVersion))
		}
	}
	return s.kubernetesGatewayVersion, nil
}

// VersionOnStartup return the version of the relevant CRD on startup.
func (s *CheckGatewayCRDsState) VersionOnStartup(name string) string {
	return s.relevantCRDDeployed[name]
}

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager) error {
	r.Client = mgr.GetClient()
	r.ShutdownFunc = r.shutdown

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		For(
			&apiextensionsv1.CustomResourceDefinition{},
			builder.WithPredicates(Predicate()),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
			RecoverPanic:            ptr.To(false),
			NeedLeaderElection:      ptr.To(false),
		}).
		Complete(r)
}

// Predicate returns the predicate to be considered for reconciliation.
func Predicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			crd, ok := e.Object.(*apiextensionsv1.CustomResourceDefinition)
			if !ok || crd == nil {
				return false
			}
			for _, name := range relevantCRDs {
				if crd.Name == name {
					return true
				}
			}
			return false
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			crd, ok := e.ObjectNew.(*apiextensionsv1.CustomResourceDefinition)
			if !ok || crd == nil {
				return false
			}
			for _, name := range relevantCRDs {
				if crd.Name == name {
					return true
				}
			}
			return false
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			crd, ok := e.Object.(*apiextensionsv1.CustomResourceDefinition)
			if !ok || crd == nil {
				return false
			}
			for _, name := range relevantCRDs {
				if crd.Name == name {
					return true
				}
			}
			return false
		},

		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

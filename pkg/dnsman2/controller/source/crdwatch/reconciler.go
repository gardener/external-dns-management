// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crdwatch

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/client-go/discovery"
	"k8s.io/utils/ptr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi"
	gatewayapiv1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi/v1"
	gatewayapiv1beta1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi/v1beta1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/istio"
	istiov1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/istio/v1"
	istiov1alpha3 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/istio/v1alpha3"
	istiov1beta1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/istio/v1beta1"
)

// Reconciler is the reconciler for the CRD watch controller.
type Reconciler struct {
	Config    config.SourceControllerConfig
	Discovery discovery.DiscoveryInterface
	Exit      func(int)
}

// Reconcile reconciles on CRD creation or deletion.
func (r *Reconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx).WithName(ControllerName)

	if err := r.handleGatewayAPI(log); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.handleIstio(log); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) handleGatewayAPI(log logr.Logger) error {
	apiVersion, err := gatewayapi.DetermineAPIVersion(r.Discovery)
	if err != nil {
		return fmt.Errorf("could not determine Gateway API version: %w", err)
	}

	switch ptr.Deref(apiVersion, "") {
	case gatewayapi.V1Beta1:
		if !gatewayapiv1beta1.Activated {
			log.V(1).Info("Found Gateway API v1beta1 CRDs, but the source controller is deactivated. Restarting for initialization...")
			r.Exit(3)
		}
	case gatewayapi.V1:
		if !gatewayapiv1.Activated {
			log.V(1).Info("Found Gateway API v1 CRDs, but the source controller is deactivated. Restarting for initialization...")
			r.Exit(3)
		}
	default:
		if gatewayapiv1beta1.Activated || gatewayapiv1.Activated {
			log.V(1).Info("Source controller for Gateway API is active, but no relevant CRDs found. Restarting for initialization...")
			r.Exit(3)
		}
	}

	return nil
}

func (r *Reconciler) handleIstio(log logr.Logger) error {
	apiVersion, err := istio.DetermineAPIVersion(r.Discovery)
	if err != nil {
		return fmt.Errorf("could not determine Istio API version: %w", err)
	}

	switch ptr.Deref(apiVersion, "") {
	case istio.V1Alpha3:
		if !istiov1alpha3.Activated {
			log.V(1).Info("Found Istio v1alpha3 CRDs, but the source controller is deactivated. Restarting for initialization...")
			r.Exit(3)
		}
	case istio.V1Beta1:
		if !istiov1beta1.Activated {
			log.V(1).Info("Found Istio v1beta1 CRDs, but the source controller is deactivated. Restarting for initialization...")
			r.Exit(3)
		}
	case istio.V1:
		if !istiov1.Activated {
			log.V(1).Info("Found Istio v1 CRDs, but the source controller is deactivated. Restarting for initialization...")
			r.Exit(3)
		}
	default:
		if istiov1alpha3.Activated || istiov1beta1.Activated || istiov1.Activated {
			log.V(1).Info("Source controller for Istio is active, but no relevant CRDs found. Restarting for initialization...")
			r.Exit(3)
		}
	}

	return nil
}

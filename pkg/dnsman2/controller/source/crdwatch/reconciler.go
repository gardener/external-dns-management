// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crdwatch

import (
	"context"
	"fmt"

	"k8s.io/client-go/discovery"
	"k8s.io/utils/ptr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi"
	gatewayapiv1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi/v1"
	gatewayapiv1beta1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi/v1beta1"
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

	apiVersion, err := gatewayapi.DetermineAPIVersion(r.Discovery)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("could not determine Gateway API version: %w", err)
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

	return reconcile.Result{}, nil
}

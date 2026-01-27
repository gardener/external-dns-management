// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crdwatch

import (
	"context"
	"fmt"
	"os"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	gatewayapiv1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi/v1"
	gatewayapiv1beta1 "github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/gatewayapi/v1beta1"
)

// Reconciler is the reconciler for CRD watch controller.
type Reconciler struct {
	Config config.SourceControllerConfig
	Client client.Client
}

// Reconcile reconciles on CRD creation or deletion.
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx).WithName(ControllerName)

	deactivated, err := isResponsibleSourceControllerDeactivated(request.Name)
	if err != nil {
		return reconcile.Result{}, err
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := r.Client.Get(ctx, request.NamespacedName, crd); err != nil {
		if apierrors.IsNotFound(err) && !deactivated {
			log.Info("Relevant CRD deleted, shutting down to deactivate source controller.", "crd", crd.Name)
			os.Exit(3)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if deactivated {
		log.Info("Relevant CRD created, shutting down to activate source controller.", "crd", crd.Name)
		os.Exit(3)
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, nil
}

func isResponsibleSourceControllerDeactivated(crdName string) (bool, error) {
	if isGatewayAPICRD(crdName) {
		return gatewayapiv1beta1.IsDeactivated() || gatewayapiv1.IsDeactivated(), nil
	}

	return false, fmt.Errorf("unexpected CRD %s", crdName)
}

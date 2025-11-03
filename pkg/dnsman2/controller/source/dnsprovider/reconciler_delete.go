// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsprovider

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

func (r *Reconciler) delete(ctx context.Context, log logr.Logger, sourceProvider *v1alpha1.DNSProvider) (reconcile.Result, error) {
	log.Info("delete")

	targetProviders, err := r.getExistingTargetProviders(ctx, sourceProvider)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := r.deleteObsoleteTargetProviders(ctx, log, sourceProvider, targetProviders, nil); err != nil {
		return reconcile.Result{}, err
	}

	if err := removeFinalizer(ctx, r.Client, sourceProvider); err != nil {
		return reconcile.Result{}, fmt.Errorf("error removing finalizer from provider %s: %w", sourceProvider.Name, err)
	}

	return reconcile.Result{}, nil
}

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

func (r *Reconciler) delete(ctx context.Context, log logr.Logger, provider *v1alpha1.DNSProvider) (reconcile.Result, error) {
	// TODO: implement/complete deletion logic

	entries := v1alpha1.DNSEntryList{}
	if err := r.Client.List(ctx, &entries, client.InNamespace(provider.Namespace), client.MatchingFields{EntryStatusProvider: client.ObjectKeyFromObject(provider).String()}); err != nil {
		return reconcile.Result{}, fmt.Errorf("error listing DNSEntries for provider %s: %w", provider.Name, err)
	}

	if len(entries.Items) > 0 {
		// TODO: handle existing DNSEntries, update provider status
		return reconcile.Result{}, fmt.Errorf("not yet implemented")
	}

	if err := removeFinalizer(ctx, r.Client, provider); err != nil {
		return reconcile.Result{}, fmt.Errorf("error removing finalizer from provider %s: %w", provider.Name, err)
	}

	return reconcile.Result{}, nil
}

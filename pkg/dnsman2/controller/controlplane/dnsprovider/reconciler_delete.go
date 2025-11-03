// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsprovider

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

func (r *Reconciler) delete(ctx context.Context, log logr.Logger, provider *v1alpha1.DNSProvider) (reconcile.Result, error) {
	log.Info("delete")

	entries := v1alpha1.DNSEntryList{}
	if err := r.Client.List(ctx, &entries, client.InNamespace(provider.Namespace), client.MatchingFields{EntryStatusProvider: client.ObjectKeyFromObject(provider).String()}); err != nil {
		return reconcile.Result{}, fmt.Errorf("error listing DNSEntries for provider %s: %w", provider.Name, err)
	}

	if len(entries.Items) > 0 {
		log.Info("provider still has DNSEntries, cannot delete", "entryCount", len(entries.Items))
		res := reconcile.Result{
			RequeueAfter: 5 * time.Minute,
		}
		if since := time.Since(provider.DeletionTimestamp.Time); since < 30*time.Second {
			res.RequeueAfter = 1 * time.Second
		} else if since > 0 && since < 30*time.Minute {
			res.RequeueAfter = since / 10
		}
		if res, err := r.handleEmptyProviderState(ctx, log, provider); !res.IsZero() || err != nil {
			return res, err
		}
		return res, r.updateStatus(ctx, provider, func(status *v1alpha1.DNSProviderStatus) error {
			status.Message = ptr.To(fmt.Sprintf("cannot delete provider, %d DNSEntries still assigned to it", len(entries.Items)))
			return nil
		})
	}

	if err := removeFinalizer(ctx, r.Client, provider); err != nil {
		return reconcile.Result{}, fmt.Errorf("error removing finalizer from provider %s: %w", provider.Name, err)
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) handleEmptyProviderState(ctx context.Context, log logr.Logger, provider *v1alpha1.DNSProvider) (reconcile.Result, error) {
	if r.state.GetProviderState(client.ObjectKeyFromObject(provider)) == nil {
		// after controller restart, reconcile to recreate provider state
		res, err := r.reconcile(ctx, log, provider)
		if !res.IsZero() || err != nil {
			return res, err
		}
		res.RequeueAfter = 1 * time.Second
		return res, nil
	}
	return reconcile.Result{}, nil
}

// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package dnsanntation

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

func (r *Reconciler) delete(ctx context.Context, log logr.Logger, annotation *v1alpha1.DNSAnnotation) (reconcile.Result, error) {
	log.Info("delete")
	r.deleteByKey(ctx, log, client.ObjectKeyFromObject(annotation))
	return reconcile.Result{}, nil
}

func (r *Reconciler) deleteByKey(ctx context.Context, log logr.Logger, nameKey client.ObjectKey) {
	if ref := r.state.DeleteByAnnotationKey(nameKey); ref != nil {
		// Re-reconcile any other DNSAnnotation for the same resourceRef so it can take over.
		if rerr := r.reconcileConflicting(ctx, log, *ref); rerr != nil {
			log.Error(rerr, "failed to re-reconcile conflicting annotations after deletion")
		}
	}
}

// reconcileConflicting re-reconciles all DNSAnnotations with the same resourceRef so one can take over.
func (r *Reconciler) reconcileConflicting(ctx context.Context, log logr.Logger, ref v1alpha1.ResourceReference) error {
	list := &v1alpha1.DNSAnnotationList{}
	if err := r.Client.List(ctx, list); err != nil {
		return fmt.Errorf("listing DNSAnnotations: %w", err)
	}
	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.ResourceRef == ref {
			if _, err := r.reconcile(ctx, log, item); err != nil {
				return err
			}
		}
	}
	return nil
}

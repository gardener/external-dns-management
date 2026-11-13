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

func (r *Reconciler) reconcile(ctx context.Context, log logr.Logger, annotation *v1alpha1.DNSAnnotation) (reconcile.Result, error) {
	log.Info("reconcile")

	ref := normalizeRef(annotation.Spec.ResourceRef, annotation.Namespace)
	if r.NamespaceRestriction && ref.Namespace != annotation.Namespace {
		err := fmt.Errorf("cross-namespace resourceRef is not allowed (DNSProviderNamespaceRestriction is enabled): annotation namespace %q, ref namespace %q", annotation.Namespace, ref.Namespace)
		log.Error(err, "rejected cross-namespace resourceRef")
		// Evict any stale state entry this annotation previously owned so it does not block others.
		r.state.DeleteByAnnotationKey(client.ObjectKeyFromObject(annotation))
		return reconcile.Result{}, r.updateStatus(ctx, annotation, func(status *v1alpha1.DNSAnnotationStatus) error {
			status.Message = err.Error()
			status.Active = false
			return nil
		})
	}

	if err := r.state.SetResourceAnnotations(ref, client.ObjectKeyFromObject(annotation), annotation.Spec.Annotations); err != nil {
		log.Info("failed to set resource annotations in state", "error", err.Error())
		return reconcile.Result{}, r.updateStatus(ctx, annotation, func(status *v1alpha1.DNSAnnotationStatus) error {
			status.Message = err.Error()
			status.Active = false
			return nil
		})
	}

	return reconcile.Result{}, r.updateStatus(ctx, annotation, func(status *v1alpha1.DNSAnnotationStatus) error {
		_, message, active := r.state.GetResourceAnnotationStatus(ref)
		if message != status.Message || active != status.Active {
			log.V(1).Info("updated annotation status", "active", active, "message", message)
		}
		status.Message = message
		status.Active = active
		return nil
	})
}

// normalizeRef defaults an empty Namespace to ownerNamespace, matching the
// comment on ResourceReference.Namespace ("Defaulted by the namespace of the containing resource").
func normalizeRef(ref v1alpha1.ResourceReference, ownerNamespace string) v1alpha1.ResourceReference {
	if ref.Namespace == "" {
		ref.Namespace = ownerNamespace
	}
	return ref
}

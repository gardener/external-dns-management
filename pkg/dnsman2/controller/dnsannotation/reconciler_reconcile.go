// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsanntation

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

func (r *Reconciler) reconcile(ctx context.Context, log logr.Logger, annotation *v1alpha1.DNSAnnotation) (reconcile.Result, error) {
	log.Info("reconcile")
	if annotation.Spec.ResourceRef.Namespace != annotation.Namespace {
		log.Info("cross-namespace annotation not allowed")
		return reconcile.Result{}, r.updateStatus(ctx, annotation, func(status *v1alpha1.DNSAnnotationStatus) error {
			status.Message = "cross-namespace annotation not allowed"
			status.Active = false
			return nil
		})
	}
	if err := r.state.SetResourceAnnotations(annotation.Spec.ResourceRef, client.ObjectKeyFromObject(annotation), annotation.Spec.Annotations); err != nil {
		log.Info("failed to set resource annotations in state", "error", err.Error())
		return reconcile.Result{}, r.updateStatus(ctx, annotation, func(status *v1alpha1.DNSAnnotationStatus) error {
			status.Message = err.Error()
			status.Active = false
			return nil
		})
	}

	return reconcile.Result{}, r.updateStatus(ctx, annotation, func(status *v1alpha1.DNSAnnotationStatus) error {
		_, message, active := r.state.GetResourceAnnotationStatus(annotation.Spec.ResourceRef)
		if message != status.Message || active != status.Active {
			log.V(1).Info("updated annotation status", "active", active, "message", message)
		}
		status.Message = message
		status.Active = active
		return nil
	})
}

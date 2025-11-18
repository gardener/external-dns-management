/*
 * SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsanntation

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// Reconciler is a reconciler for DNSAnnotation resources on the control plane.
type Reconciler struct {
	Client      client.Client
	Clock       clock.Clock
	Recorder    record.EventRecorder
	Config      config.DNSAnnotationControllerConfig
	SourceClass string

	state state.AnnotationState
}

// Reconcile reconciles DNSAnnotation resources.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx).WithName(ControllerName)

	annotation := &v1alpha1.DNSAnnotation{}
	if err := r.Client.Get(ctx, req.NamespacedName, annotation); err != nil {
		if apierrors.IsNotFound(err) {
			r.deleteByKey(req.NamespacedName)
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	if annotation.DeletionTimestamp != nil {
		return r.delete(ctx, log, annotation)
	} else {
		return r.reconcile(ctx, log, annotation)
	}
}

func (r *Reconciler) updateStatus(ctx context.Context, annotation *v1alpha1.DNSAnnotation, modify func(status *v1alpha1.DNSAnnotationStatus) error) error {
	patch := client.MergeFrom(annotation.DeepCopy())
	if err := modify(&annotation.Status); err != nil {
		return err
	}

	return r.Client.Status().Patch(ctx, annotation, patch)
}

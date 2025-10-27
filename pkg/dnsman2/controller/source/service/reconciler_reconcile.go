// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
)

func (r *Reconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	service *corev1.Service,
) (
	reconcile.Result,
	error,
) {
	log.Info("reconcile")

	var input *common.DNSSpecInput
	if isRelevantService(service, r.Class) {
		var err error
		input, err = common.GetDNSSpecInputForService(log, service)
		if err != nil {
			r.Recorder.Eventf(service, corev1.EventTypeWarning, "Invalid", "%s", err)
			return reconcile.Result{}, err
		}
	}

	return r.DoReconcile(ctx, log, service, input)
}

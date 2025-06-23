/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsentry

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/dnsentry/lookup"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// Reconciler is a reconciler for DNSProvider resources on the control plane.
type Reconciler struct {
	Client                     client.Client
	Config                     config.DNSEntryControllerConfig
	Clock                      clock.Clock
	Namespace                  string
	Class                      string
	defaultCNAMELookupInterval int64

	state           *state.State
	lookupProcessor lookup.LookupProcessor
}

// Reconcile reconciles DNSProvider resources.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx).WithName(ControllerName).WithName(req.String())

	entry := &v1alpha1.DNSEntry{}
	if err := r.Client.Get(ctx, req.NamespacedName, entry); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	er := entryReconciliation{
		EntryContext: common.EntryContext{
			Client: r.Client,
			Clock:  r.Clock,
			Ctx:    logr.NewContext(ctx, log),
			Log:    log,
			Entry:  entry,
		},
		namespace:                  r.Namespace,
		class:                      r.Class,
		lookupProcessor:            r.lookupProcessor,
		state:                      r.state,
		defaultCNAMELookupInterval: r.defaultCNAMELookupInterval,
	}
	res := er.reconcile()
	return res.Result, res.Err
}

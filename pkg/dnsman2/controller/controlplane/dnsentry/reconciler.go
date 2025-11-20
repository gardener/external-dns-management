/*
 * SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsentry

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/jellydator/ttlcache/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsentry/lookup"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

const defaultReconciliationDelayAfterUpdate = 5 * time.Second

// Reconciler is a reconciler for DNSEntry resources on the control plane.
type Reconciler struct {
	Client                         client.Client
	Config                         config.DNSEntryControllerConfig
	Clock                          clock.Clock
	Namespace                      string
	Class                          string
	defaultCNAMELookupInterval     int64
	reconciliationDelayAfterUpdate time.Duration

	state           *state.State
	lookupProcessor lookup.LookupProcessor
	lastUpdate      *ttlcache.Cache[client.ObjectKey, struct{}]
}

// Reconcile reconciles DNSEntry resources.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx).WithName(ControllerName)

	r.lastUpdate.DeleteExpired()
	if r.lastUpdate.Has(req.NamespacedName) {
		// Delay reconciliation for a short time as authoritative DNS servers may not be updated immediately.
		// The same update may be triggered again otherwise.
		log.V(1).Info("Entry was already updated recently, postponing reconciliation")
		return reconcile.Result{RequeueAfter: r.reconciliationDelayAfterUpdate}, nil
	}

	entry := &v1alpha1.DNSEntry{}
	if err := r.Client.Get(ctx, req.NamespacedName, entry); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, ptr.Deref(r.Config.ReconciliationTimeout, metav1.Duration{Duration: 2 * time.Minute}).Duration)
	er := entryReconciliation{
		EntryContext: common.EntryContext{
			Client: r.Client,
			Clock:  r.Clock,
			Ctx:    logr.NewContext(ctxWithTimeout, log),
			Log:    log,
			Entry:  entry,
		},
		namespace:                  r.Namespace,
		class:                      r.Class,
		lookupProcessor:            r.lookupProcessor,
		state:                      r.state,
		defaultCNAMELookupInterval: r.defaultCNAMELookupInterval,
		lastUpdate:                 r.lastUpdate,
	}
	res := er.reconcile()
	if res.Err != nil {
		log.Error(res.Err, "reconciliation failed")
	} else if res.Result.RequeueAfter > 0 {
		log.Info("reconciliation scheduled to be retried", "requeueAfter", res.Result.RequeueAfter)
	} else {
		log.Info("reconciliation succeeded")
	}
	cancel()
	return res.Result, res.Err
}

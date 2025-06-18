// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

// EntryContext holds context and references for a DNSEntry reconciliation.
type EntryContext struct {
	Client client.Client
	Clock  clock.Clock
	Ctx    context.Context
	Log    logr.Logger
	Entry  *v1alpha1.DNSEntry
}

// StatusUpdater returns a new EntryStatusUpdater for this EntryContext.
func (ec *EntryContext) StatusUpdater() *EntryStatusUpdater {
	return &EntryStatusUpdater{EntryContext: *ec}
}

// ReconcileResult wraps a controller-runtime reconcile result and error.
type ReconcileResult struct {
	Result reconcile.Result
	Err    error
}

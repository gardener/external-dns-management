/*
 * SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package dnsanntation

import (
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	dnsman2controller "github.com/gardener/external-dns-management/pkg/dnsman2/controller"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// ControllerName is the name of this controller.
const ControllerName = "dnsannotation"

// AddToManager adds Reconciler to the given manager.
func (r *Reconciler) AddToManager(mgr manager.Manager) error {
	r.Client = mgr.GetClient()
	if r.Clock == nil {
		r.Clock = clock.RealClock{}
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor(ControllerName + "-controller")
	}
	r.state = state.GetState().GetAnnotationState()

	return builder.
		ControllerManagedBy(mgr).
		Named(ControllerName).
		For(
			&v1alpha1.DNSAnnotation{},
			builder.WithPredicates(
				dnsman2controller.DNSClassPredicate(dns.NormalizeClass(r.Config.Class)),
			),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: ptr.Deref(r.Config.Controllers.DNSAnnotation.ConcurrentSyncs, 2),
			SkipNameValidation:      r.Config.Controllers.DNSAnnotation.SkipNameValidation,
		}).
		Complete(r)
}

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gardener/gardener/pkg/controllerutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
)

// entryStatusUpdater is a utility to update the status of a DNSEntry resource and handle finalizers.
type entryStatusUpdater struct {
	entryContext
}

func (u *entryStatusUpdater) updateStatusWithoutProvider() reconcileResult {
	if res := u.updateStatus(func(status *v1alpha1.DNSEntryStatus) error {
		status.Provider = nil
		status.ObservedGeneration = u.entry.Generation
		if len(status.Targets) > 0 && status.Zone != nil {
			status.State = v1alpha1.StateStale
		} else {
			status.State = v1alpha1.StateError
			status.ProviderType = nil
		}
		status.Message = ptr.To("no matching DNS provider found")
		return nil
	}); res != nil {
		return *res
	}
	if len(u.entry.Status.Targets) == 0 {
		if res := u.removeFinalizer(); res != nil {
			return *res
		}
	}
	return reconcileResult{} // No provider or zone found, nothing to do
}

func (u *entryStatusUpdater) updateStatusWithProvider(targetsData *newTargetsData) reconcileResult {
	if res := u.updateStatus(func(status *v1alpha1.DNSEntryStatus) error {
		status.Provider = &targetsData.providerKey.Name
		status.ProviderType = ptr.To(targetsData.providerType)
		status.Zone = &targetsData.zoneID.ID
		name := targetsData.dnsSet.Name
		rp := targetsData.routingPolicy
		status.DNSName = ptr.To(name.DNSName)
		if name.SetIdentifier != "" && rp != nil {
			status.RoutingPolicy = &v1alpha1.RoutingPolicy{
				Type:          string(rp.Type),
				Parameters:    rp.Parameters,
				SetIdentifier: name.SetIdentifier,
			}
		} else {
			status.RoutingPolicy = nil
		}
		status.Targets = TargetsToStrings(targetsData.targets)
		if len(targetsData.targets) > 0 {
			status.TTL = ptr.To(targetsData.targets[0].GetTTL())
		} else {
			status.TTL = nil
		}

		status.ObservedGeneration = u.entry.Generation
		status.State = v1alpha1.StateReady
		if len(targetsData.warnings) > 0 {
			status.Message = ptr.To(fmt.Sprintf("reconciled with warnings: %s", strings.Join(targetsData.warnings, ", ")))
		} else {
			status.Message = ptr.To("dns entry active")
		}
		return nil
	}); res != nil {
		return *res
	}

	if len(u.entry.Status.Targets) == 0 {
		if res := u.removeFinalizer(); res != nil {
			return *res
		}
	}

	return reconcileResult{}
}

func (u *entryStatusUpdater) updateStatus(modify func(status *v1alpha1.DNSEntryStatus) error) *reconcileResult {
	patch := client.MergeFrom(u.entry.DeepCopy())
	oldStatus := u.entry.Status.DeepCopy()

	if err := modify(&u.entry.Status); err != nil {
		u.log.Error(err, "failed to modify status")
		return &reconcileResult{err: err}
	}
	if !reflect.DeepEqual(oldStatus, &u.entry.Status) {
		u.entry.Status.LastUpdateTime = &metav1.Time{Time: u.clock.Now()}
	}

	if err := u.client.Status().Patch(u.ctx, u.entry, patch); err != nil {
		u.log.Error(err, "failed to update status")
		return &reconcileResult{err: err}
	}
	return nil
}

func (u *entryStatusUpdater) updateStatusFailed(state string, err error) *reconcileResult {
	return u.updateStatus(func(status *v1alpha1.DNSEntryStatus) error {
		status.Message = ptr.To(err.Error())
		status.State = state
		status.ObservedGeneration = u.entry.Generation
		return nil
	})
}

func (u *entryStatusUpdater) updateStatusInvalid(err error) *reconcileResult {
	return u.updateStatusFailed(v1alpha1.StateInvalid, err)
}

func (u *entryStatusUpdater) failWithStatusStale(err error) reconcileResult {
	if res := u.updateStatusFailed(v1alpha1.StateStale, err); res != nil {
		return *res
	}
	return reconcileResult{result: reconcile.Result{Requeue: true}, err: fmt.Errorf("failed with state stale: %w", err)}
}

func (u *entryStatusUpdater) failWithStatusError(err error) reconcileResult {
	if res := u.updateStatusFailed(v1alpha1.StateError, err); res != nil {
		return *res
	}
	return reconcileResult{err: fmt.Errorf("failed to reconcile: %w", err)}
}

func (u *entryStatusUpdater) addFinalizer() *reconcileResult {
	if err := controllerutils.AddFinalizers(u.ctx, u.client, u.entry, dns.FinalizerCompound); err != nil {
		u.log.Error(err, "failed to add finalizer")
		return &reconcileResult{err: err}
	}
	return nil
}

func (u *entryStatusUpdater) removeFinalizer() *reconcileResult {
	if err := controllerutils.RemoveFinalizers(u.ctx, u.client, u.entry, dns.FinalizerCompound); err != nil {
		u.log.Error(err, "failed to remove finalizer")
		return &reconcileResult{err: err}
	}
	return nil
}

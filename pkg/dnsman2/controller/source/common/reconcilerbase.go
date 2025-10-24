/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// ReconcilerBase is base for source reconcilers.
type ReconcilerBase struct {
	Client             client.Client
	ControlPlaneClient client.Client
	Recorder           record.EventRecorder
	Class              string
	GVK                schema.GroupVersionKind
	Config             config.SourceControllerConfig
	State              state.AnnotationState
}

// DoReconcile reconciles for given object and dnsSpecInput.
func (r *ReconcilerBase) DoReconcile(ctx context.Context, log logr.Logger, obj client.Object, dnsSpecInput *DNSSpecInput) (reconcile.Result, error) {
	ownedEntries, err := r.getExistingOwnedDNSEntries(ctx, obj)
	if err != nil {
		return reconcile.Result{}, err
	}
	if dnsSpecInput == nil || dnsSpecInput.Names.IsEmpty() {
		return r.DoDelete(ctx, log, obj)
	}

	newEntries := map[string]*dnsv1alpha1.DNSEntry{}
	for _, name := range dnsSpecInput.Names.ToSlice() {
		var matchingEntry *dnsv1alpha1.DNSEntry
		for _, ownedEntry := range ownedEntries {
			if ownedEntry.Spec.DNSName == name {
				matchingEntry = &ownedEntry
				break
			}
		}
		if matchingEntry != nil {
			newEntries[name] = matchingEntry
		} else {
			newEntries[name] = r.newDNSEntry(obj)
		}
	}

	if err := r.deleteObsoleteOwnedDNSEntries(ctx, log, obj, ownedEntries, maps.Values(newEntries)); err != nil {
		return reconcile.Result{}, err
	}

	for _, name := range dnsSpecInput.Names.ToSlice() {
		if err := r.createOrUpdateDNSEntry(ctx, log, obj, dnsSpecInput, name, newEntries[name]); err != nil {
			return reconcile.Result{}, err
		}
	}

	ref := BuildResourceReference(r.GVK, obj)
	return reconcile.Result{}, r.State.UpdateStatus(ctx, r.Client, ref, true)
}

func (r *ReconcilerBase) createOrUpdateDNSEntry(
	ctx context.Context,
	log logr.Logger,
	obj client.Object,
	dnsSpecInput *DNSSpecInput,
	dnsName string,
	entry *dnsv1alpha1.DNSEntry,
) error {
	modifier := func() error {
		modifyEntryFor(entry, r.Config, dnsSpecInput, dnsName)
		return nil
	}
	if entry.Name == "" {
		if err := modifier(); err != nil {
			return fmt.Errorf("failed to apply modifier: %w", err)
		}
		if err := r.ControlPlaneClient.Create(ctx, entry); err != nil {
			return fmt.Errorf("failed to create DNSEntry: %w", err)
		}
		log.Info("created DNSEntry", "name", entry.Name)
		r.Recorder.Eventf(obj, corev1.EventTypeNormal, "DNSEntryCreated", "Created DNSEntry: %s", entry.Name) // TODO: check former reason/message
		return nil
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.ControlPlaneClient, entry, modifier)
	if err != nil {
		return fmt.Errorf("failed to patch DNSEntry %s: %w", client.ObjectKeyFromObject(entry), err)
	}
	if result == controllerutil.OperationResultUpdated {
		log.Info("updated DNSEntry", "name", entry.Name)
		r.Recorder.Eventf(obj, corev1.EventTypeNormal, "DNSEntryUpdated", "Updated DNSEntry: %s", entry.Name) // TODO: check former reason/message
	}
	return nil
}

// DoDelete performs delete reconciliation for given object.
func (r *ReconcilerBase) DoDelete(ctx context.Context, log logr.Logger, obj client.Object) (reconcile.Result, error) {
	log.Info("cleanup")

	ownedEntries, err := r.getExistingOwnedDNSEntries(ctx, obj)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := r.deleteObsoleteOwnedDNSEntries(ctx, log, obj, ownedEntries, nil); err != nil {
		return reconcile.Result{}, err
	}

	ref := BuildResourceReference(r.GVK, obj)
	return reconcile.Result{}, r.State.UpdateStatus(ctx, r.Client, ref, false)
}

func (r *ReconcilerBase) getExistingOwnedDNSEntries(ctx context.Context, owner metav1.Object) ([]dnsv1alpha1.DNSEntry, error) {
	candidates := &dnsv1alpha1.DNSEntryList{}
	if err := r.ControlPlaneClient.List(ctx, candidates, client.InNamespace(r.targetNamespace(owner))); err != nil {
		return nil, fmt.Errorf("failed to list owned DNSEntries for %s %s: %w", r.GVK.Kind, owner, err)
	}

	var ownedEntries []dnsv1alpha1.DNSEntry
	for _, candidate := range candidates.Items {
		if r.IsOwnedByController(&candidate, owner) {
			ownedEntries = append(ownedEntries, candidate)
		}
	}
	return ownedEntries, nil
}

// IsOwnedByController checks whether the given DNSEntry is owned by the given owner.
func (r *ReconcilerBase) IsOwnedByController(entry *dnsv1alpha1.DNSEntry, owner metav1.Object) bool {
	return r.buildOwnerData(owner).HasOwner(entry, ptr.Deref(r.Config.TargetClusterID, ""))
}

func (r *ReconcilerBase) buildOwnerData(owner metav1.Object) OwnerData {
	return OwnerData{
		Object:    owner,
		ClusterID: ptr.Deref(r.Config.SourceClusterID, ""),
		GVK:       r.GVK,
	}
}

func (r *ReconcilerBase) deleteObsoleteOwnedDNSEntries(
	ctx context.Context,
	log logr.Logger,
	obj client.Object,
	ownedEntries []dnsv1alpha1.DNSEntry,
	entriesToKeep []*dnsv1alpha1.DNSEntry,
) error {
outer:
	for _, ownedEntry := range ownedEntries {
		for _, entryToKeep := range entriesToKeep {
			if ownedEntry.Name == entryToKeep.Name {
				continue outer
			}
		}
		if err := r.ControlPlaneClient.Delete(ctx, &ownedEntry); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete obsolete owned DNSEntry %s: %w", client.ObjectKeyFromObject(&ownedEntry), err)
		}
		log.Info("deleted obsolete owned DNSEntry", "name", ownedEntry.Name)
		r.Recorder.Eventf(obj, corev1.EventTypeNormal, "DNSEntryDeleted", "Deleted DNSEntry: %s", ownedEntry.Name) // TODO: check former reason/message
	}
	return nil
}

func (r *ReconcilerBase) newDNSEntry(owner metav1.Object) *dnsv1alpha1.DNSEntry {
	namespace := r.targetNamespace(owner)
	entry := &dnsv1alpha1.DNSEntry{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: strings.ToLower(fmt.Sprintf("%s%s-%s-", ptr.Deref(r.Config.TargetNamePrefix, ""), owner.GetName(), r.GVK.Kind)),
			Namespace:    namespace,
		},
	}
	r.buildOwnerData(owner).AddOwner(entry, ptr.Deref(r.Config.TargetClusterID, ""))
	return entry
}

func (r *ReconcilerBase) targetNamespace(owner metav1.Object) string {
	if ns := ptr.Deref(r.Config.TargetNamespace, ""); ns != "" {
		return ns
	}
	return owner.GetNamespace()
}

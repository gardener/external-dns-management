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

	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// SourceReconciler is base for source reconcilers.
type SourceReconciler[SourceObject client.Object] struct {
	actuator           SourceActuator[SourceObject]
	Log                logr.Logger
	Client             client.Client
	ControlPlaneClient client.Client
	Recorder           RecorderWithDeduplication
	GVK                schema.GroupVersionKind
	Config             config.SourceControllerConfig
	FinalizerName      string
	SourceClass        string
	TargetClass        string
	State              state.AnnotationState
}

// NewSourceReconciler creates a new SourceReconciler for given actuator.
func NewSourceReconciler[SourceObject client.Object](actuator SourceActuator[SourceObject]) *SourceReconciler[SourceObject] {
	return &SourceReconciler[SourceObject]{
		actuator: actuator,
		GVK:      actuator.GetGVK(),
		State:    state.GetState().GetAnnotationState(),
	}
}

// DoReconcile reconciles for given object and dnsSpecInput.
func (r *SourceReconciler[SourceObject]) DoReconcile(ctx context.Context, obj client.Object, dnsSpecInput *DNSSpecInput) (reconcile.Result, error) {
	ownedEntries, err := r.getExistingOwnedDNSEntries(ctx, obj)
	if err != nil {
		return reconcile.Result{}, err
	}
	if dnsSpecInput == nil || dnsSpecInput.Names.IsEmpty() {
		return r.DoDelete(ctx, obj)
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

	if err := r.deleteObsoleteOwnedDNSEntries(ctx, obj, ownedEntries, maps.Values(newEntries)); err != nil {
		return reconcile.Result{}, err
	}

	for _, name := range dnsSpecInput.Names.ToSlice() {
		if err := r.createOrUpdateDNSEntry(ctx, obj, dnsSpecInput, name, newEntries[name]); err != nil {
			return reconcile.Result{}, err
		}
	}

	if len(newEntries) > 0 {
		if err := controllerutils.AddFinalizers(ctx, r.Client, obj, r.FinalizerName); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to add finalizer from %s %s: %w", r.GVK.Kind, obj.GetName(), err)
		}
	} else {
		if err := controllerutils.RemoveFinalizers(ctx, r.Client, obj, r.FinalizerName); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to remove finalizer from %s %s: %w", r.GVK.Kind, obj.GetName(), err)
		}
	}

	ref := BuildResourceReference(r.GVK, obj)
	return reconcile.Result{}, r.State.UpdateStatus(ctx, r.Client, ref, true)
}

func (r *SourceReconciler[SourceObject]) createOrUpdateDNSEntry(
	ctx context.Context,
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
		r.Log.Info("created DNSEntry", "name", entry.Name)
		r.Recorder.Eventf(obj, corev1.EventTypeNormal, "DNSEntryCreated", "%s: created entry %s in control plane", entry.Spec.DNSName, entry.Name)
		return nil
	}

	result, err := controllerutil.CreateOrPatch(ctx, r.ControlPlaneClient, entry, modifier)
	if err != nil {
		return fmt.Errorf("failed to patch DNSEntry %s: %w", client.ObjectKeyFromObject(entry), err)
	}
	if result == controllerutil.OperationResultUpdated {
		r.Log.Info("updated DNSEntry", "name", entry.Name)
		r.Recorder.Eventf(obj, corev1.EventTypeNormal, "DNSEntryUpdated", "%s: updated entry %s in control plane", entry.Spec.DNSName, entry.Name)
	}
	return nil
}

// DoDelete performs delete reconciliation for given object.
func (r *SourceReconciler[SourceObject]) DoDelete(ctx context.Context, obj client.Object) (reconcile.Result, error) {
	r.Log.Info("cleanup")

	ownedEntries, err := r.getExistingOwnedDNSEntries(ctx, obj)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := r.deleteObsoleteOwnedDNSEntries(ctx, obj, ownedEntries, nil); err != nil {
		return reconcile.Result{}, err
	}

	if err := controllerutils.RemoveFinalizers(ctx, r.Client, obj, r.FinalizerName); err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to remove finalizer from %s %s: %w", r.GVK.Kind, obj.GetName(), err)
	}

	ref := BuildResourceReference(r.GVK, obj)
	return reconcile.Result{}, r.State.UpdateStatus(ctx, r.Client, ref, false)
}

func (r *SourceReconciler[SourceObject]) getExistingOwnedDNSEntries(ctx context.Context, owner metav1.Object) ([]dnsv1alpha1.DNSEntry, error) {
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
func (r *SourceReconciler[SourceObject]) IsOwnedByController(entry *dnsv1alpha1.DNSEntry, owner metav1.Object) bool {
	return r.buildOwnerData(owner).HasOwner(entry, ptr.Deref(r.Config.TargetClusterID, ""))
}

func (r *SourceReconciler[SourceObject]) buildOwnerData(owner metav1.Object) OwnerData {
	return OwnerData{
		Object:    owner,
		ClusterID: ptr.Deref(r.Config.SourceClusterID, ""),
		GVK:       r.GVK,
	}
}

func (r *SourceReconciler[SourceObject]) deleteObsoleteOwnedDNSEntries(
	ctx context.Context,
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
		r.Log.Info("delete obsolete owned DNSEntry", "name", ownedEntry.Name)
		r.Recorder.DedupEventf(obj, corev1.EventTypeNormal, "DNSEntryDeleted", "%s: deleted entry %s in control plane", ownedEntry.Spec.DNSName, ownedEntry.Name)
	}
	return nil
}

func (r *SourceReconciler[SourceObject]) newDNSEntry(owner metav1.Object) *dnsv1alpha1.DNSEntry {
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

func (r *SourceReconciler[SourceObject]) targetNamespace(owner metav1.Object) string {
	if ns := ptr.Deref(r.Config.TargetNamespace, ""); ns != "" {
		return ns
	}
	return owner.GetNamespace()
}

// Reconcile reconciles source objects using the actuator.
func (r *SourceReconciler[SourceObject]) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	sourceObject := r.actuator.NewSourceObject()
	if err := r.Client.Get(ctx, req.NamespacedName, sourceObject); err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.V(1).Info("Object is gone, stop reconciling")
			r.actuator.OnDelete(sourceObject)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	if sourceObject.GetDeletionTimestamp() != nil {
		r.actuator.OnDelete(sourceObject)
		return r.DoDelete(ctx, sourceObject)
	}

	return r.actuator.ReconcileSourceObject(ctx, r, sourceObject)
}

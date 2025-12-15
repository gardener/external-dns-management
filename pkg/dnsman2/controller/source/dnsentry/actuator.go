// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsentry

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// ControllerName is the name of this controller.
const ControllerName = "dnsentry-source"

// Actuator is the actuator for DNSEntry source objects.
type Actuator struct {
}

var _ common.SourceActuator[*dnsv1alpha1.DNSEntry] = &Actuator{}

// ReconcileSourceObject reconciles the given DNSEntry object.
func (a *Actuator) ReconcileSourceObject(
	ctx context.Context,
	r *common.SourceReconciler[*dnsv1alpha1.DNSEntry],
	entry *dnsv1alpha1.DNSEntry,
) (
	reconcile.Result,
	error,
) {
	r.Log.Info("reconcile")

	var input *common.DNSSpecInput
	if a.IsRelevantSourceObject(r, entry) {
		input = getDNSSpecInputForDNSEntry(entry)
	}

	res, err := r.DoReconcile(ctx, entry, input)
	if err != nil {
		r.Recorder.DedupEventf(entry, corev1.EventTypeWarning, "ReconcileError", "%s", err)
		return res, err
	}
	if targetKey := entry.Annotations[dns.AnnotationTargetEntry]; targetKey != "" {
		objectKey := getTargetEntryObjectKey(targetKey)
		if objectKey == nil {
			r.Log.Error(nil, "Invalid target DNSEntry annotation", "value", targetKey)
			return res, nil
		}
		targetEntry := &dnsv1alpha1.DNSEntry{}
		if err = r.ControlPlaneClient.Get(ctx, *objectKey, targetEntry); client.IgnoreNotFound(err) != nil {
			r.Log.Error(err, "Could not get target DNSEntry", "key", targetKey)
			return res, err
		}
		if err == nil && targetEntry.DeletionTimestamp == nil {
			patch := client.MergeFrom(entry.DeepCopy())
			entry.Status = *targetEntry.Status.DeepCopy()
			entry.Status.ObservedGeneration = entry.Generation
			if err := r.Client.Status().Patch(ctx, entry, patch); err != nil {
				r.Log.Error(err, "Could not update status")
				return res, err
			}
		}
	}
	return res, nil
}

// ControllerName returns the name of this controller.
func (a *Actuator) ControllerName() string {
	return ControllerName
}

// FinalizerLocalName returns the local name of the finalizer.
func (a *Actuator) FinalizerLocalName() string {
	return "dnsentry-source"
}

// GetGVK returns the GVK of DNSEntry.
func (a *Actuator) GetGVK() schema.GroupVersionKind {
	return dnsv1alpha1.SchemeGroupVersion.WithKind("DNSEntry")
}

// IsRelevantSourceObject checks whether the given DNSEntry is relevant for DNS management.
func (a *Actuator) IsRelevantSourceObject(r *common.SourceReconciler[*dnsv1alpha1.DNSEntry], entry *dnsv1alpha1.DNSEntry) bool {
	if entry == nil {
		return false
	}
	return dns.EquivalentClass(entry.GetAnnotations()[dns.AnnotationClass], r.SourceClass)
}

// NewSourceObject creates a new DNSEntry object.
func (r *Actuator) NewSourceObject() *dnsv1alpha1.DNSEntry {
	return &dnsv1alpha1.DNSEntry{}
}

// ShouldSetTargetEntryAnnotation indicates whether the target DNSEntry annotation should be set on the source object.
func (a *Actuator) ShouldSetTargetEntryAnnotation() bool {
	// used to update status from target DNSEntry during reconciliation
	return true
}

func getDNSSpecInputForDNSEntry(entry *dnsv1alpha1.DNSEntry) *common.DNSSpecInput {
	names := utils.NewUniqueStrings()
	if entry.Spec.DNSName == "" {
		return nil
	}
	names.Add(entry.Spec.DNSName)
	targets := utils.NewUniqueStrings()
	for _, name := range entry.Spec.Targets {
		targets.Add(name)
	}
	text := utils.NewUniqueStrings()
	for _, name := range entry.Spec.Text {
		text.Add(name)
	}

	return &common.DNSSpecInput{
		Names:                     names,
		Targets:                   targets,
		Text:                      text,
		IPStack:                   entry.GetAnnotations()[dns.AnnotationIPStack],
		ResolveTargetsToAddresses: entry.Spec.ResolveTargetsToAddresses,
		CNameLookupInterval:       entry.Spec.CNameLookupInterval,
		TTL:                       entry.Spec.TTL,
		RoutingPolicy:             entry.Spec.RoutingPolicy,
		Ignore:                    entry.GetAnnotations()[dns.AnnotationIgnore],
	}
}

func getTargetEntryObjectKey(targetKey string) *client.ObjectKey {
	parts := strings.Split(targetKey, "/")
	if len(parts) != 2 {
		return nil
	}
	return &client.ObjectKey{Namespace: parts[0], Name: parts[1]}
}

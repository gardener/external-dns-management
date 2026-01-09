/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// OwnerData contains the information about an owner object.
type OwnerData struct {
	Object    metav1.Object
	ClusterID string
	GVK       schema.GroupVersionKind
}

func (o OwnerData) String() string {
	return o.AsAnnotationRef("")
}

// AsAnnotationRef returns the owner reference in the format used in the
// annotation `resources.gardener.cloud/owners`.
func (o OwnerData) AsAnnotationRef(targetClusterID string) string {
	basicRef := fmt.Sprintf("%s/%s/%s/%s", o.GVK.Group, o.GVK.Kind, o.Object.GetNamespace(), o.Object.GetName())
	if targetClusterID == o.ClusterID {
		return basicRef
	}
	return o.ClusterID + ":" + basicRef
}

// AddOwner adds an owner reference to the given object. If the owner is from
// the same cluster and namespace, a proper owner reference is added. Otherwise,
// the owner reference is stored in an annotation.
func (o OwnerData) AddOwner(obj metav1.Object, clusterID string) bool {
	return o.addOwner(obj, clusterID, false)
}

// HasOwner checks whether the given owner is already present in the object's
// owner references or in the annotation `resources.gardener.cloud/owners`.
// It returns true if the owner reference or annotation is already present,
// false otherwise.
func (o OwnerData) HasOwner(obj metav1.Object, clusterID string) bool {
	return !o.addOwner(obj, clusterID, true)
}

func (o OwnerData) addOwner(obj metav1.Object, clusterID string, checkOnly bool) bool {
	if o.Object.GetNamespace() == obj.GetNamespace() && o.ClusterID == clusterID {
		ownerRef := metav1.NewControllerRef(o.Object, o.GVK)
		for _, r := range obj.GetOwnerReferences() {
			if ownerRef.UID == r.UID {
				return false
			}
		}
		if !checkOnly {
			obj.SetOwnerReferences(append(obj.GetOwnerReferences(), *ownerRef))
		}
		return true
	} else {
		// maintain foreign references via annotations
		var (
			ref  = o.AsAnnotationRef(clusterID)
			refs = GetAnnotatedOwners(obj)
		)
		if slices.Contains(refs, ref) {
			return false
		}
		if !checkOnly {
			refs = append(refs, ref)
			utils.SetAnnotation(obj, dns.AnnotationOwners, strings.Join(refs, ","))
		}
		return true
	}
}

// RemoveOwner removes the given owner from the object's owner references or
// from the annotation `resources.gardener.cloud/owners`.
// It returns true if an owner reference or annotation was removed, false otherwise.
func (o OwnerData) RemoveOwner(obj metav1.Object, clusterID string) bool {
	if o.Object.GetNamespace() == obj.GetNamespace() && o.ClusterID == clusterID {
		ownerRef := metav1.NewControllerRef(o.Object, o.GVK)
		var (
			newRefs []metav1.OwnerReference
			deleted bool
		)
		for _, r := range obj.GetOwnerReferences() {
			if ownerRef.UID == r.UID {
				deleted = true
			} else {
				newRefs = append(newRefs, r)
			}
		}
		obj.SetOwnerReferences(newRefs)
		return deleted
	} else {
		// maintain foreign references via annotations
		var (
			ref     = o.AsAnnotationRef(clusterID)
			refs    = GetAnnotatedOwners(obj)
			deleted bool
			newRefs []string
		)
		for _, r := range refs {
			if ref == r {
				deleted = true
			} else {
				newRefs = append(newRefs, r)
			}
		}
		utils.SetAnnotation(obj, dns.AnnotationOwners, strings.Join(newRefs, ","))
		return deleted
	}
}

// EntryOwnerData contains the information about the owner type of DNSEntry.
type EntryOwnerData struct {
	Config config.SourceControllerConfig
	GVK    schema.GroupVersionKind
}

// IsRelevantEntry checks whether the given DNSEntry has an owner of the given
// GroupVersionKind. If sameNamespaceAndCluster is true, only the owner references
// are checked. Otherwise, the annotation `resources.gardener.cloud/owners`
// is checked.
func (d EntryOwnerData) IsRelevantEntry(entry *dnsv1alpha1.DNSEntry) bool {
	if !dns.EquivalentClass(entry.Annotations[dns.AnnotationClass], ptr.Deref(d.Config.TargetClass, "")) {
		return false
	}
	if d.Config.TargetNamespace != nil && entry.Namespace != *d.Config.TargetNamespace {
		return false
	}

	owners := d.GetOwnerObjectKeys(entry)
	return len(owners) > 0
}

// GetOwnerObjectKeys returns the list of owner object keys for the given
func (d EntryOwnerData) GetOwnerObjectKeys(obj metav1.Object) []client.ObjectKey {
	var ownerKeys []client.ObjectKey

	if d.Config.TargetNamespace == nil && reflect.DeepEqual(d.Config.SourceClusterID, d.Config.TargetClusterID) {
		for _, r := range obj.GetOwnerReferences() {
			if d.GVK.Kind == r.Kind && d.GVK.GroupVersion().String() == r.APIVersion {
				ownerKeys = append(ownerKeys, client.ObjectKey{Namespace: obj.GetNamespace(), Name: r.Name})
			}
		}
		return ownerKeys
	}

	prefix := ""
	if d.Config.SourceClusterID != nil {
		prefix = *d.Config.SourceClusterID + ":"
	}
	prefix += fmt.Sprintf("%s/%s/", d.GVK.Group, d.GVK.Kind)
	for _, ownerRef := range GetAnnotatedOwners(obj) {
		if after, ok := strings.CutPrefix(ownerRef, prefix); ok {
			suffix := after
			nameParts := strings.SplitN(suffix, "/", 2)
			if len(nameParts) != 2 {
				continue
			}
			ownerKeys = append(ownerKeys, client.ObjectKey{Namespace: nameParts[0], Name: nameParts[1]})
		}
	}
	return ownerKeys
}

// GetAnnotatedOwners returns the list of owner references stored in the
// annotation `resources.gardener.cloud/owners`.
func GetAnnotatedOwners(obj metav1.Object) []string {
	s := obj.GetAnnotations()[dns.AnnotationOwners]
	if s == "" {
		return nil
	}
	var owners []string
	for o := range strings.SplitSeq(s, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			owners = append(owners, o)
		}
	}
	return owners
}

// ForResourceMapDNSEntry returns a function that maps a DNSEntry to its owning resource(s).
func ForResourceMapDNSEntry(gkv schema.GroupVersionKind) func(context.Context, client.Object) []reconcile.Request {
	kind := gkv.Kind
	apiVersion := gkv.GroupVersion().String()
	prefix := gkv.Group + "/" + gkv.Kind + "/"
	return func(_ context.Context, obj client.Object) []reconcile.Request {
		entry, ok := obj.(*dnsv1alpha1.DNSEntry)
		if !ok {
			return nil
		}
		if entry.OwnerReferences != nil {
			for _, ownerRef := range entry.OwnerReferences {
				if ownerRef.Kind == kind && ownerRef.APIVersion == apiVersion {
					return []reconcile.Request{{
						NamespacedName: client.ObjectKey{
							Namespace: entry.Namespace,
							Name:      ownerRef.Name,
						},
					}}
				}
			}
			return nil
		}

		var requests []reconcile.Request
		owners := GetAnnotatedOwners(entry)
		for _, owner := range owners {
			parts := strings.SplitN(owner, ":", 2)
			suffix := parts[len(parts)-1]
			oldLen := len(suffix)
			suffix = strings.TrimPrefix(suffix, prefix)
			if oldLen == len(suffix) {
				continue
			}
			nameParts := strings.SplitN(suffix, "/", 2)
			if len(nameParts) != 2 {
				continue
			}
			requests = append(requests, reconcile.Request{
				NamespacedName: client.ObjectKey{
					Namespace: nameParts[0],
					Name:      nameParts[1],
				},
			})
		}
		return requests
	}
}

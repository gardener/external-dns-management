/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package common

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type OwnerData struct {
	Object    metav1.Object
	ClusterID string
	GVK       schema.GroupVersionKind
}

func (o OwnerData) String() string {
	return o.AsAnnotationRef("")
}

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
func AddOwner(obj metav1.Object, clusterID string, owner OwnerData) bool {
	return addOwner(obj, clusterID, owner, false)
}

// HasOwner checks whether the given owner is already present in the object's
// owner references or in the annotation `resources.gardener.cloud/owners`.
// It returns true if the owner reference or annotation is already present,
// false otherwise.
func HasOwner(obj metav1.Object, clusterID string, owner OwnerData) bool {
	return !addOwner(obj, clusterID, owner, true)
}

func addOwner(obj metav1.Object, clusterID string, owner OwnerData, checkOnly bool) bool {
	if owner.Object.GetNamespace() == obj.GetNamespace() && owner.ClusterID == clusterID {
		ownerRef := metav1.NewControllerRef(owner.Object, owner.GVK)
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
			ref  = owner.AsAnnotationRef(clusterID)
			refs = GetAnnotatedOwners(obj)
		)
		for _, r := range refs {
			if ref == r {
				return false
			}
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
func RemoveOwner(obj metav1.Object, clusterID string, owner OwnerData) bool {
	if owner.Object.GetNamespace() == obj.GetNamespace() && owner.ClusterID == clusterID {
		ownerRef := metav1.NewControllerRef(owner.Object, owner.GVK)
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
			ref     = owner.AsAnnotationRef(clusterID)
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

// GetAnnotatedOwners returns the list of owner references stored in the
// annotation `resources.gardener.cloud/owners`.
func GetAnnotatedOwners(obj metav1.Object) []string {
	s := obj.GetAnnotations()[dns.AnnotationOwners]
	if s == "" {
		return nil
	}
	var owners []string
	for _, o := range strings.Split(s, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			owners = append(owners, o)
		}
	}
	return owners
}

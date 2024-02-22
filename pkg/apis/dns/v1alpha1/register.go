// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/gardener/external-dns-management/pkg/apis/dns"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	Version   = "v1alpha1"
	GroupName = dns.GroupName

	DNSOwnerKind            = "DNSOwner"
	DNSProviderKind         = "DNSProvider"
	DNSEntryKind            = "DNSEntry"
	DNSLockKind             = "DNSLock"
	DNSAnnotationKind       = "DNSAnnotation"
	DNSHostedZonePolicyKind = "DNSHostedZonePolicy"

	RemoteAccessCertificateKind = "RemoteAccessCertificate"
)

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: dns.GroupName, Version: Version}

// Kind takes an unqualified kind and returns back a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resources and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// Adds the list of known types to Scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&DNSOwner{},
		&DNSOwnerList{},
		&DNSProvider{},
		&DNSProviderList{},
		&DNSEntry{},
		&DNSEntryList{},
		&DNSAnnotation{},
		&DNSLock{},
		&DNSLockList{},
		&DNSAnnotationList{},
		&DNSHostedZonePolicy{},
		&DNSHostedZonePolicyList{},
		&RemoteAccessCertificate{},
		&RemoteAccessCertificateList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

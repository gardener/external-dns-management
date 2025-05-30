// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0
// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	internalinterfaces "github.com/gardener/external-dns-management/pkg/client/dns/informers/externalversions/internalinterfaces"
)

// Interface provides access to all the informers in this group version.
type Interface interface {
	// DNSAnnotations returns a DNSAnnotationInformer.
	DNSAnnotations() DNSAnnotationInformer
	// DNSEntries returns a DNSEntryInformer.
	DNSEntries() DNSEntryInformer
	// DNSHostedZonePolicies returns a DNSHostedZonePolicyInformer.
	DNSHostedZonePolicies() DNSHostedZonePolicyInformer
	// DNSProviders returns a DNSProviderInformer.
	DNSProviders() DNSProviderInformer
}

type version struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a new Interface.
func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &version{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}

// DNSAnnotations returns a DNSAnnotationInformer.
func (v *version) DNSAnnotations() DNSAnnotationInformer {
	return &dNSAnnotationInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// DNSEntries returns a DNSEntryInformer.
func (v *version) DNSEntries() DNSEntryInformer {
	return &dNSEntryInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// DNSHostedZonePolicies returns a DNSHostedZonePolicyInformer.
func (v *version) DNSHostedZonePolicies() DNSHostedZonePolicyInformer {
	return &dNSHostedZonePolicyInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// DNSProviders returns a DNSProviderInformer.
func (v *version) DNSProviders() DNSProviderInformer {
	return &dNSProviderInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"github.com/gardener/controller-manager-library/pkg/resources"
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var DNSHostedZonePolicyType = (*api.DNSHostedZonePolicy)(nil)

type DNSHostedZonePolicyObject struct {
	resources.Object
}

func (this *DNSHostedZonePolicyObject) DNSHostedZonePolicyObject() *api.DNSHostedZonePolicy {
	return this.Data().(*api.DNSHostedZonePolicy)
}

func DNSHostedZonePolicy(o resources.Object) *DNSHostedZonePolicyObject {
	if o.IsA(DNSHostedZonePolicyType) {
		return &DNSHostedZonePolicyObject{o}
	}
	return nil
}

func (this *DNSHostedZonePolicyObject) Spec() *api.DNSHostedZonePolicySpec {
	return &this.DNSHostedZonePolicyObject().Spec
}

func (this *DNSHostedZonePolicyObject) Status() *api.DNSHostedZonePolicyStatus {
	return &this.DNSHostedZonePolicyObject().Status
}

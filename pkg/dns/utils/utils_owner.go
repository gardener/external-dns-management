// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"github.com/gardener/controller-manager-library/pkg/resources"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var DNSOwnerType = (*api.DNSOwner)(nil)

type DNSOwnerObject struct {
	resources.Object
}

func (this *DNSOwnerObject) DNSOwner() *api.DNSOwner {
	return this.Data().(*api.DNSOwner)
}

func DNSOwner(o resources.Object) *DNSOwnerObject {
	if o.IsA(DNSOwnerType) {
		return &DNSOwnerObject{o}
	}
	return nil
}

func (this *DNSOwnerObject) Spec() *api.DNSOwnerSpec {
	return &this.DNSOwner().Spec
}

func (this *DNSOwnerObject) GetOwnerId() string {
	return this.DNSOwner().Spec.OwnerId
}

func (this *DNSOwnerObject) IsEnabled() bool {
	a := this.DNSOwner().Spec.Active
	return a == nil || *a
}

func (this *DNSOwnerObject) IsActive() bool {
	return this.IsEnabled()
}

func (this *DNSOwnerObject) GetCounts() map[string]int {
	return this.DNSOwner().Status.Entries.ByType
}

func (this *DNSOwnerObject) GetCount() int {
	return this.DNSOwner().Status.Entries.Amount
}

func (this *DNSOwnerObject) Status() *api.DNSOwnerStatus {
	return &this.DNSOwner().Status
}

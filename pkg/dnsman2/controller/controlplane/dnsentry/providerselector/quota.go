// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package providerselector

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsprovider"
)

// ListEntriesForProvider returns the DNSEntry resources currently assigned to the specified provider.
// Only entries that have status.provider set are returned (i.e., entries that have been successfully provisioned).
func ListEntriesForProvider(ctx context.Context, c client.Client, namespace string, providerKey client.ObjectKey) ([]v1alpha1.DNSEntry, error) {
	entryList := &v1alpha1.DNSEntryList{}
	if err := c.List(ctx, entryList,
		client.InNamespace(namespace),
		client.MatchingFields{dnsprovider.EntryStatusProvider: providerKey.String()},
	); err != nil {
		return nil, err
	}
	return entryList.Items, nil
}

// quotaExceededError is returned when a provider has reached its entries quota.
type quotaExceededError struct {
	providerKey client.ObjectKey
	quota       int32
}

func (e *quotaExceededError) Error() string {
	return fmt.Sprintf("provider %s has reached its entries quota (max=%d)", e.providerKey, e.quota)
}

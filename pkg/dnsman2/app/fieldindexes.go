// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"

	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/controlplane/dnsprovider"
)

// AddAllFieldIndexesToCluster adds all field indices to the given manager.
func AddAllFieldIndexesToCluster(ctx context.Context, cluster cluster.Cluster) error {
	for _, fn := range []func(context.Context, client.FieldIndexer) error{
		dnsprovider.AddEntryStatusProvider,
	} {
		if err := fn(ctx, cluster.GetFieldIndexer()); err != nil {
			return err
		}
	}

	return nil
}

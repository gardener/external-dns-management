/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cluster

import (
	"context"

	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

///////////////////////////////////////////////////////////////////////////////

func CreateClustersFromDefaults(ctx context.Context, logger logger.LogContext, cfg *areacfg.Config, names utils.StringSet) (Clusters, error) {
	return registry.GetDefinitions().CreateClusters(ctx, logger, cfg, NewSchemeCache(), names)
}

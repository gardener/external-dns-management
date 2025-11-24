/*
 * SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package source

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/common"
	"github.com/gardener/external-dns-management/pkg/dnsman2/controller/source/service"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/state"
)

// AddToManager adds all source controllers to the manager.
func AddToManager(mgr manager.Manager, controlPlaneCluster cluster.Cluster, cfg *config.DNSManagerConfiguration) error {
	if err := (&service.Reconciler{
		ReconcilerBase: common.ReconcilerBase{
			Config:        cfg.Controllers.Source,
			FinalizerName: dns.ClassSourceFinalizer(dns.NormalizeClass(config.GetSourceClass(cfg)), "service-dns"),
			SourceClass:   config.GetSourceClass(cfg),
			TargetClass:   config.GetTargetClass(cfg),
			State:         state.GetState().GetAnnotationState(),
		},
	}).AddToManager(mgr, controlPlaneCluster); err != nil {
		return fmt.Errorf("failed adding source Service controller: %w", err)
	}

	return nil
}

/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package reconcile

import (
	"github.com/gardener/controller-manager-library/pkg/logger"
)

func SetupReconciler(r Interface) error {
	if s, ok := r.(SetupInterface); ok {
		return s.Setup()
	}
	if s, ok := r.(LegacySetupInterface); ok {
		s.Setup()
	}
	return nil
}

func StartReconciler(r Interface) error {
	if s, ok := r.(StartInterface); ok {
		return s.Start()
	}
	if s, ok := r.(LegacyStartInterface); ok {
		s.Start()
	}
	return nil
}

func CleanupReconciler(logger logger.LogContext, n string, r Interface) error {
	if c, ok := r.(CleanupInterface); ok {
		logger.Infof("cleanup reconciler %q", n)
		err := c.Cleanup()
		if err != nil {
			logger.Warnf("  cleanup of reconciler %q failed: %s", n, err)
		}
	}
	return nil
}

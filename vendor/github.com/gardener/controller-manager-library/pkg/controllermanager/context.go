/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controllermanager

import (
	"context"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
)

var ctx_controllermanager = ctxutil.NewValueKey(config.OPTION_SOURCE, (*ControllerManager)(nil))

func GetControllerManager(ctx context.Context) *ControllerManager {
	return ctx_controllermanager.Get(ctx).(*ControllerManager)
}

var ctx_extension = ctxutil.NewValueKey("extension", (*extension.Extension)(nil))

func GetExtension(ctx context.Context) *extension.Extension {
	return ctx_extension.Get(ctx).(*extension.Extension)
}

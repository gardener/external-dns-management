/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
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

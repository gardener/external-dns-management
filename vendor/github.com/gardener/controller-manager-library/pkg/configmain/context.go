/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved.
 * This file is licensed under the Apache Software License, v. 2 except as noted
 * otherwise in the LICENSE file
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

package configmain

import (
	"context"
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

var typekey reflect.Type

func init() {
	typekey, _ = utils.TypeKey((*Config)(nil))
}

func Get(ctx context.Context) *Config {
	cfg := ctx.Value(typekey)
	if cfg == nil {
		return nil
	}
	return cfg.(*Config)
}

func WithConfig(ctx context.Context, config *Config) (context.Context, *Config) {
	if config == nil {
		config = NewConfig()
	}
	return context.WithValue(ctx, typekey, config), config
}

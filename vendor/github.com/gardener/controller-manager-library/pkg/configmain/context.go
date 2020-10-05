/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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

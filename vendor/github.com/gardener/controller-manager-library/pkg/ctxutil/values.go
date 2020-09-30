/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package ctxutil

import (
	"context"
	"reflect"
)

type ValueKey interface {
	Name() string
	WithValue(ctx context.Context, value interface{}) context.Context
	Get(ctx context.Context) interface{}
}

type valueKey struct {
	name string
	key  reflect.Type
}

func NewValueKey(name string, proto interface{}) ValueKey {
	t := reflect.TypeOf(proto)
	return &valueKey{
		name: name,
		key:  t,
	}
}

func (this *valueKey) Name() string {
	return this.name
}

func (this *valueKey) WithValue(ctx context.Context, value interface{}) context.Context {
	return context.WithValue(ctx, this.key, value)
}

func (this *valueKey) Get(ctx context.Context) interface{} {
	return ctx.Value(this.key)
}

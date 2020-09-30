/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package logger

import (
	"context"
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

var ctxkey reflect.Type

func init() {
	ctxkey, _ = utils.TypeKey((*LogContext)(nil))
}

func Get(ctx context.Context) LogContext {
	cur := ctx.Value(ctxkey)
	if cur != nil {
		return cur.(LogContext)
	} else {
		return New()
	}
}

func Set(ctx context.Context, log LogContext) context.Context {
	return context.WithValue(ctx, ctxkey, log)
}

func WithLogger(ctx context.Context, key, value string) (context.Context, LogContext) {
	log := Get(ctx).NewContext(key, value)
	return Set(ctx, log), log
}

func CErrorf(ctx context.Context, msgfmt string, args ...interface{}) {
	Get(ctx).Errorf(msgfmt, args...)
}
func CWarnf(ctx context.Context, msgfmt string, args ...interface{}) {
	Get(ctx).Warnf(msgfmt, args...)
}
func CInfof(ctx context.Context, msgfmt string, args ...interface{}) {
	Get(ctx).Infof(msgfmt, args...)
}
func CDebugf(ctx context.Context, msgfmt string, args ...interface{}) {
	Get(ctx).Debugf(msgfmt, args...)
}

func CError(ctx context.Context, msg ...interface{}) {
	Get(ctx).Error(msg...)
}
func CWarn(ctx context.Context, msg ...interface{}) {
	Get(ctx).Warn(msg...)
}
func CInfo(ctx context.Context, msg ...interface{}) {
	Get(ctx).Info(msg...)
}
func CDebug(ctx context.Context, msg ...interface{}) {
	Get(ctx).Debug(msg...)
}

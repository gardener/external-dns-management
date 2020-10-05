/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package ctxutil

import (
	"context"
)

var cancelkey = ""

func CancelContext(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	return context.WithValue(ctx, &cancelkey, cancel)
}

func Cancel(ctx context.Context) {
	ctx.Value(&cancelkey).(context.CancelFunc)()
}

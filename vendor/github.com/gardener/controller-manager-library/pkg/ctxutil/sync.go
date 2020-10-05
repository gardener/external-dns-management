/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package ctxutil

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

var synckey = ""

type WaitGroup struct {
	name string
	sync.WaitGroup
}

func (this *WaitGroup) WaitWithTimeout(duration time.Duration, desc ...string) {
	if duration <= 0 {
		this.Wait()
	} else {
		shutdown, cancel := context.WithCancel(context.Background())
		timer := time.NewTimer(duration)

		go func() {
			this.Wait()
			cancel()
		}()
		select {
		case <-shutdown.Done():
		case <-timer.C:
			msg := ""
			if this.name != "" {
				msg = this.name + ": "
			}
			if len(desc) > 0 {
				for _, d := range desc {
					msg += d
				}
				msg += ": "
			}

			fmt.Printf("*** %swait timed out -> generating stack traces\n", msg)
			buf := make([]byte, 1<<16)
			stackSize := runtime.Stack(buf, true)
			fmt.Printf("%s\n", string(buf[0:stackSize]))
			cancel()
		}
	}
}

func NewWaitGroup(name string) *WaitGroup {
	return &WaitGroup{name: name}
}

////////////////////////////////////////////////////////////////////////////////

func WaitGroupContext(ctx context.Context, desc ...string) context.Context {
	name := ""
	if len(desc) > 0 {
		for _, d := range desc {
			name += d
		}
	}
	return context.WithValue(ctx, &synckey, NewWaitGroup(name))
}

func get_wg(ctx context.Context) *WaitGroup {
	return ctx.Value(&synckey).(*WaitGroup)
}

func WaitGroupAdd(ctx context.Context) {
	get_wg(ctx).Add(1)
}

func WaitGroupDone(ctx context.Context) {
	get_wg(ctx).Done()
}

func WaitGroupWait(ctx context.Context, duration time.Duration, desc ...string) {
	get_wg(ctx).WaitWithTimeout(duration, desc...)
}

func WaitGroupRun(ctx context.Context, f func()) {
	WaitGroupAdd(ctx)
	go func() {
		defer WaitGroupDone(ctx)
		f()
	}()
}

func WaitGroupRunAndCancelOnExit(ctx context.Context, f func()) {
	WaitGroupAdd(ctx)
	go func() {
		defer Cancel(ctx)
		defer WaitGroupDone(ctx)
		f()
	}()
}

func WaitGroupRunUntilCancelled(ctx context.Context, f func()) {
	WaitGroupRun(ctx, func() { wait.Until(f, time.Second, ctx.Done()) })
}

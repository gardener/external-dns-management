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

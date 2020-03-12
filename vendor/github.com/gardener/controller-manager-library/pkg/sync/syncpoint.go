/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved.
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

package sync

import (
	"context"
	"sync"
)

////////////////////////////////////////////////////////////////////////////////

type WaitLock interface {
	Wait(ctx context.Context) bool
	Setup(ctx context.Context) WaitLock
	Release()
}

type waitLock chan struct{}

func (this waitLock) Wait(ctx context.Context) bool {
	select {
	case <-this:
		return true
	case <-ctx.Done():
		return false
	}
}
func (this waitLock) Release() {
	this <- struct{}{}
}

func (this waitLock) Setup(ctx context.Context) WaitLock {
	return this
}

type SyncPoint struct {
	lock        sync.Mutex
	initialized bool
	waiting     []WaitLock
}

func (this *SyncPoint) IsReached() bool {
	this.lock.Lock()
	this.lock.Unlock()
	return this.initialized
}

func (this *SyncPoint) Reach() {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.initialized = true
	for _, w := range this.waiting {
		w.Release()
	}
	this.waiting = nil
}

func (this *SyncPoint) Sync(ctx context.Context) bool {
	this.lock.Lock()
	if this.initialized {
		this.lock.Unlock()
		return true
	}

	wait := make(waitLock).Setup(ctx)
	this.waiting = append(this.waiting, wait)
	this.lock.Unlock()
	return wait.Wait(ctx)
}

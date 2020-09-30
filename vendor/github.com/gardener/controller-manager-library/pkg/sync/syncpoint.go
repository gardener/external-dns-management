/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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

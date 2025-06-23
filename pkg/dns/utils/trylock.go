// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"time"

	"golang.org/x/sync/semaphore"
)

// TryLock is a lock supporting both `Lock` and `TryLock` by wrapping a weighted semaphore.
type TryLock struct {
	lock *semaphore.Weighted
	ctx  context.Context
}

// NewTryLock creates a lock based on a weighted semaphore.
func NewTryLock(ctx ...context.Context) *TryLock {
	var c context.Context
	switch len(ctx) {
	case 0:
		c = context.TODO()
	case 1:
		c = ctx[0]
	default:
		panic("multiple context not allowed")
	}
	return &TryLock{lock: semaphore.NewWeighted(1), ctx: c}
}

// Lock acquires the lock blocking until resource is available
func (l *TryLock) Lock() error {
	if err := l.lock.Acquire(l.ctx, 1); err != nil {
		return err
	}
	return nil
}

// TryLock tries to acquire the resource and returns true if successful.
func (l *TryLock) TryLock() bool {
	return l.lock.TryAcquire(1)
}

// TryLockSpinning tries to acquire the resource for some time and returns true if successful.
func (l *TryLock) TryLockSpinning(spinTime time.Duration) bool {
	end := time.Now().Add(spinTime)
	waitTime := 200 * time.Microsecond
	for {
		if l.TryLock() {
			return true
		}
		delta := time.Until(end)
		if waitTime > delta {
			time.Sleep(delta)
			return l.TryLock()
		}
		time.Sleep(waitTime)
		waitTime = 11 * waitTime / 10
	}
}

// Unlock releases the resource
func (l *TryLock) Unlock() {
	l.lock.Release(1)
}

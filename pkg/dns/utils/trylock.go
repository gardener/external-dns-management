/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
func NewTryLock() *TryLock {
	return &TryLock{lock: semaphore.NewWeighted(1), ctx: context.TODO()}
}

// Lock acquires the lock blocking until resource is available
func (l *TryLock) Lock() {
	if err := l.lock.Acquire(l.ctx, 1); err != nil {
		panic(err)
	}
}

// TryLock tries to acquire the resource and returns true if successful.
func (l *TryLock) TryLock() bool {
	if !l.lock.TryAcquire(1) {
		return false
	}
	return true
}

// TryLockSpinning tries to acquire the resource for some time and returns true if successful.
func (l *TryLock) TryLockSpinning(spinTime time.Duration) bool {
	end := time.Now().Add(spinTime)
	waitTime := 200 * time.Microsecond
	for {
		if l.TryLock() {
			return true
		}
		delta := end.Sub(time.Now())
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

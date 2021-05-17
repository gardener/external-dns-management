/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package utils

import (
	"sync"
)

type Locker interface {
	Lock() func()
	Unlock()
}

type Mutex struct {
	sync.Mutex
}

func (this *Mutex) Lock() func() {
	this.Mutex.Lock()
	return this.Unlock
}

type RWMutex struct {
	sync.RWMutex
}

func (this *RWMutex) Lock() func() {
	this.RWMutex.Lock()
	return this.Unlock
}

func (this *RWMutex) RLock() func() {
	this.RWMutex.RLock()
	return this.RUnlock
}

// RLocker returns a Locker interface that implements
// the Lock and Unlock methods by calling rw.RLock and rw.RUnlock.
func (this *RWMutex) RLocker() Locker {
	return (*rlocker)(this)
}

type rlocker RWMutex

func (r *rlocker) Lock() func() { return (*RWMutex)(r).RLock() }
func (r *rlocker) Unlock()      { (*RWMutex)(r).RUnlock() }

/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package sync

import (
	"sync"
)

///////////////////////////////////////////////////////////////////////////////

type Locker interface {
	Lock()
	Unlock()
}

type TestLocker interface {
	Locker
	TestAndLock() bool
}

type RWLock interface {
	Locker
	RLock()
	RUnlock()

	TestAndLock() bool
	TestAndRLock() bool

	RLocker() TestLocker
}

///////////////////////////////////////////////////////////////////////////////

type queue struct {
	next *block
	last **block
}

func newQueue() *queue {
	q := &queue{}
	q.last = &q.next
	return q
}

func (q *queue) enqueue(writer bool) *block {
	b := newBlock(writer)
	*q.last = b
	q.last = &b.next
	return b
}

func (q *queue) empty() bool {
	return q.next == nil
}

func (q *queue) dequeue() *block {
	b := q.next
	if b != nil {
		q.next = b.next
		if q.next == nil {
			q.last = &q.next
		}
	}
	return b
}

///////////////////////////////////////////////////////////////////////////////

type block struct {
	lock   sync.Mutex
	writer bool
	next   *block
}

func newBlock(writer bool) *block {
	b := &block{writer: writer}
	b.lock.Lock()
	return b
}

func (b *block) wait() {
	b.lock.Lock()
}

func (b *block) wakeup() {
	b.lock.Unlock()
}

///////////////////////////////////////////////////////////////////////////////

type rlocker struct {
	lock *rwlock
}

func (r *rlocker) Lock() {
	r.lock.RLock()
}

func (r *rlocker) Unlock() {
	r.lock.RUnlock()
}

func (r *rlocker) TestAndLock() bool {
	return r.lock.TestAndRLock()
}

///////////////////////////////////////////////////////////////////////////////

type rwlock struct {
	lock      sync.Mutex
	queue     *queue
	locked    int
	writelock bool
}

func NewRWLock() *rwlock {
	l := &rwlock{queue: newQueue()}
	return l
}

func (l *rwlock) RLocker() TestLocker {
	return &rlocker{l}
}

func (l *rwlock) Lock() {
	l.LockN(nil)
}

func (l *rwlock) TestAndLock() bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.locked > 0 {
		return false
	}
	// fmt.Printf("- test lock\n")
	l.locked = 1
	l.writelock = true
	return true
}

func (l *rwlock) LockN(notify chan<- struct{}) {
	l.lock.Lock()
	if l.locked > 0 {
		l.wait(true, notify)
	}
	// fmt.Printf("- lock\n")
	l.locked = 1
	l.writelock = true
	l.lock.Unlock()
}

func (l *rwlock) Unlock() {
	l.unlock(true)
}

func (l *rwlock) RLock() {
	l.RLockN(nil)
}

func (l *rwlock) TestAndRLock() bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	if !l.queue.empty() || l.writelock {
		return false
	}
	// fmt.Printf("- test read lock\n")
	l.locked++
	l.writelock = false
	return true
}

func (l *rwlock) RLockN(notify chan<- struct{}) {
	l.lock.Lock()
	if !l.queue.empty() || l.writelock {
		l.wait(false, notify)
	}
	// fmt.Printf("- read lock\n")
	l.locked++
	l.writelock = false
	l.next(false)
}

func (l *rwlock) RUnlock() {
	l.unlock(false)
}

func (l *rwlock) wait(writer bool, notify chan<- struct{}) {
	b := l.queue.enqueue(writer)
	l.lock.Unlock()
	if notify != nil {
		// fmt.Printf("- notify block\n")
		notify <- struct{}{}
	}
	b.wait()
}

func (l *rwlock) unlock(writer bool) {
	l.lock.Lock()
	if l.locked == 0 {
		panic("unlocking unlocked lock")
	}
	if l.writelock != writer {
		panic("Unlock for wrong lock type")
	}
	// fmt.Printf("- unlock %t (%d)\n", writer, l.locked)
	l.writelock = false
	l.locked--
	l.next(writer || l.locked == 0)
}

func (l *rwlock) next(allkinds bool) {
	if !l.queue.empty() && (l.queue.next.writer == allkinds || allkinds) {
		// fmt.Printf("- wakeup\n")
		l.queue.dequeue().wakeup()
	} else {
		l.lock.Unlock()
	}
}

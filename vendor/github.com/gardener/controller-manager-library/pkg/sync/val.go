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

package sync

import (
	"fmt"
)

type locker struct {
	lock        func()
	unlock      func()
	testandlock func() bool
}

func (l *locker) Lock() {
	l.lock()
}

func (l *locker) TestAndLock() bool {
	return l.testandlock()
}

func (l *locker) Unlock() {
	l.unlock()
}

///////////////////////////////////////////////////////////////////////////////

// golint: ignore
var aborted = fmt.Errorf("go routine aborted")

type Runner struct {
	name        string
	env         *Env
	lock        TestLocker
	control     chan struct{}
	blocked     chan struct{}
	locked      chan struct{}
	shouldblock bool
}

func (r *Runner) Step() error {
	fmt.Printf("step %s\n", r.name)
	r.control <- struct{}{}
	return nil
}

func (r *Runner) NoLock() error {
	fmt.Printf("no test and lock %s\n", r.name)
	if r.lock.TestAndLock() {
		return fmt.Errorf("%s was lockable", r.name)
	}
	return nil
}

func (r *Runner) Blocked() error {
	fmt.Printf("blocked %s\n", r.name)
	r.shouldblock = true
	select {
	case <-r.env.shutdown:
		return aborted
	case <-r.locked:
		return fmt.Errorf("%s was not blocked", r.name)
	case _, ok := <-r.blocked:
		if ok {
			fmt.Printf("ok blocked %s\n", r.name)
			return nil
		}
		return aborted
	}
}
func (r *Runner) Locked() error {
	fmt.Printf("locked %s\n", r.name)
	if !r.shouldblock {
		ok := true
		select {
		case _, ok = <-r.blocked:
			if !ok {
				return aborted
			}
			return fmt.Errorf("%s was blocked", r.name)
		default:
		}
	}
	select {
	case <-r.env.shutdown:
		return aborted
	case <-r.locked:
		return nil
	}
}

func (r *Runner) run() {
	fmt.Printf("%s start\n", r.name)
	if r.env.wait(r.control) {
		fmt.Printf("%s exec lock\n", r.name)
		r.lock.Lock()
		fmt.Printf("%s locked\n", r.name)
		close(r.locked)
		if r.env.wait(r.control) {
			fmt.Printf("%s unlock\n", r.name)
			r.lock.Unlock()
		}
	}
	fmt.Printf("%s done\n", r.name)
}

///////////////////////////////////////////////////////////////////////////////

type Env struct {
	rwlock   *rwlock
	shutdown chan struct{}
}

func (e *Env) wait(c chan struct{}) bool {
	select {
	case <-c:
		return true
	case <-e.shutdown:
		return false
	}
}

func NewEnv() *Env {
	e := &Env{NewRWLock(), make(chan struct{})}
	return e
}

func (e Env) Locker(notify chan<- struct{}) TestLocker {
	return &locker{
		func() { e.rwlock.LockN(notify) },
		func() { e.rwlock.Unlock() },
		func() bool { return e.rwlock.TestAndLock() },
	}
}

func (e Env) RLocker(notify chan<- struct{}) TestLocker {
	return &locker{
		func() { e.rwlock.RLockN(notify) },
		func() { e.rwlock.RUnlock() },
		func() bool { return e.rwlock.TestAndRLock() },
	}
}

func (e *Env) NewRunner(name string, locker func(notify chan<- struct{}) TestLocker) *Runner {
	blocked := make(chan struct{}, 2)
	control := make(chan struct{}, 2)
	locked := make(chan struct{}, 2)

	lock := locker(blocked)

	r := &Runner{name, e, lock, control, blocked, locked, false}
	go r.run()
	return r
}

func (e *Env) TestSeq(name string, step ...func() error) bool {
	for i, s := range step {
		err := s()
		if err != nil {
			if err != aborted {
				fmt.Printf("%s: step %d: failed: %s \n", name, i+1, err)
				close(e.shutdown)
				return false
			} else {
				fmt.Printf("%s: step %d: aborted", name, i+1)
			}
		}
	}
	fmt.Printf("%s: succeeded\n", name)
	return true
}

func Test() {
	Test1()
	Test2()
	Test3()
	Test4()
	Test5()
	/*
	 */
}

func Test1() {
	e := NewEnv()
	r1 := e.NewRunner("writer1", e.Locker)
	r2 := e.NewRunner("writer2", e.Locker)
	e.TestSeq("Write Lock",
		r1.Step,
		r1.Locked,
		r2.Step,
		r2.Blocked,
		r1.Step,
		r2.Locked,
		r2.Step,
	)
}
func Test2() {
	e := NewEnv()
	r1 := e.NewRunner("reader1", e.RLocker)
	r2 := e.NewRunner("reader2", e.RLocker)
	e.TestSeq("Read Lock",
		r1.Step,
		r1.Locked,
		r2.Step,
		r2.Locked,
		r1.Step,
		r2.Step,
	)
}
func Test3() {
	e := NewEnv()
	w1 := e.NewRunner("writer1", e.Locker)
	r1 := e.NewRunner("reader1", e.RLocker)
	r2 := e.NewRunner("reader2", e.RLocker)
	e.TestSeq("Write Read Lock",
		w1.Step,
		w1.Locked,
		r1.Step,
		r1.Blocked,
		r2.Step,
		r2.Blocked,
		w1.Step,
		r1.Locked,
		r2.Locked,
		r1.Step,
		r2.Step,
	)
}
func Test4() {
	e := NewEnv()
	w1 := e.NewRunner("writer1", e.Locker)
	r1 := e.NewRunner("reader1", e.RLocker)
	r2 := e.NewRunner("reader2", e.RLocker)
	r3 := e.NewRunner("reader3", e.RLocker)
	e.TestSeq("Read Write Read Lock",
		r1.Step,
		r1.Locked,
		w1.Step,
		w1.Blocked,
		r2.Step,
		r2.Blocked,
		r3.Step,
		r3.Blocked,
		r1.Step,
		w1.Locked,
		w1.Step,
		r2.Locked,
		r2.Locked,
	)
}
func Test5() {
	e := NewEnv()
	w1 := e.NewRunner("writer1", e.Locker)
	w2 := e.NewRunner("writer2", e.Locker)
	r1 := e.NewRunner("reader1", e.RLocker)
	r2 := e.NewRunner("reader2", e.RLocker)
	r3 := e.NewRunner("reader3", e.RLocker)
	e.TestSeq("Write Read Read Write Read Lock",
		w1.Step,
		w1.Locked,
		r1.Step,
		r1.Blocked,
		r2.Step,
		r2.Blocked,
		w2.Step,
		w2.Blocked,
		w1.Step,
		r1.Locked,
		r2.Locked,

		r3.NoLock,
		r3.Step,
		r3.Blocked,

		r1.Step,
		w2.NoLock,
		r2.Step,

		w2.Locked,
		w2.Step,
		r3.Locked,
	)
}

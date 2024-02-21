// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Timer struct {
	lock  sync.Mutex
	done  <-chan struct{}
	stop  chan struct{}
	timer *time.Timer
	exec  func(*Timer)
}

func NewTimer(ctx context.Context, exec func(*Timer)) *Timer {
	return &Timer{
		done: ctx.Done(),
		stop: make(chan struct{}),
		exec: exec,
	}
}

func (this *Timer) Next(d time.Duration) {
	this.lock.Lock()
	defer this.lock.Unlock()

	if this.timer == nil {
		this.timer = time.NewTimer(d)
		go this.process()
	} else {
		this.timer.Stop()
		this.timer.Reset(d)
	}
}

func (this *Timer) Stop() {
	this.lock.Lock()
	defer this.lock.Unlock()
	if this.timer != nil {
		this.timer.Stop()
		close(this.stop)
	}
}

func (this *Timer) process() {
loop:
	for {
		select {
		case <-this.done:
			break loop
		case <-this.stop:
			break loop
		case _, ok := <-this.timer.C:
			if !ok {
				break loop
			}
			go this.exec(this)
		}
	}
}

type ScheduleExecutor interface {
	Execute(key ScheduleKey)
}

type ScheduleExecutorFunction func(key ScheduleKey)

func (this ScheduleExecutorFunction) Execute(key ScheduleKey) {
	this(key)
}

type ScheduleKey interface{}

type ScheduleEntry struct {
	key ScheduleKey
	due time.Time
}

type scheduleEntry struct {
	ScheduleEntry
	next *scheduleEntry
	prev **scheduleEntry
}

func (this ScheduleEntry) Key() ScheduleKey {
	return this.key
}

func (this ScheduleEntry) Due() time.Time {
	return this.due
}

func (this ScheduleEntry) String() string {
	return fmt.Sprintf("%s: %s", this.key, this.due.Format(time.RFC3339))
}

type Schedule struct {
	lock sync.Mutex
	ctx  context.Context
	exec ScheduleExecutor

	timer *Timer

	entries map[ScheduleKey]*scheduleEntry
	next    *scheduleEntry
}

func NewSchedule(ctx context.Context, exec ScheduleExecutor) *Schedule {
	sched := &Schedule{
		entries: map[ScheduleKey]*scheduleEntry{},
		ctx:     ctx,
		exec:    exec,
	}
	sched.timer = NewTimer(ctx, sched.execute)
	return sched
}

func (this *Schedule) Reset() {
	this.lock.Lock()
	defer this.lock.Unlock()

	this.timer.Stop()
	this.timer = NewTimer(this.ctx, this.execute)
}

func (this *Schedule) execute(timer *Timer) {
	this.lock.Lock()
	defer this.lock.Unlock()

	if this.timer != timer {
		return
	}
	now := time.Now()
	for this.next != nil && !this.next.due.After(now) {
		go this.exec.Execute(this.next.key)
		delete(this.entries, this.next.key)
		this.next.prev = &this.next
		this.next = this.next.next
	}
	if this.next != nil {
		timer.Next(this.next.due.Sub(now))
	}
}

func (this *Schedule) List() []ScheduleKey {
	this.lock.Lock()
	defer this.lock.Unlock()

	list := []ScheduleKey{}
	next := this.next
	for next != nil {
		list = append(list, next.key)
		next = next.next
	}
	return list
}

func (this *Schedule) ListSchedule() []ScheduleEntry {
	this.lock.Lock()
	defer this.lock.Unlock()

	list := []ScheduleEntry{}
	next := this.next
	for next != nil {
		list = append(list, next.ScheduleEntry)
		next = next.next
	}
	return list
}

func (this *Schedule) Delete(key ScheduleKey) {
	this.lock.Lock()
	defer this.lock.Unlock()

	old := this.entries[key]
	if old != nil {
		if old.next != nil {
			old.next.prev = old.prev
		}
		*old.prev = old.next
	}
}

func (this *Schedule) ScheduleAfter(key ScheduleKey, due time.Duration) {
	this.Schedule(key, time.Now().Add(due))
}

func (this *Schedule) Schedule(key ScheduleKey, due time.Time) {
	this.lock.Lock()
	defer this.lock.Unlock()

	select {
	case _, ok := <-this.ctx.Done():
		if !ok {
			panic("schedule is closed")
		}
	default:
	}

	var cur time.Time

	if this.next != nil {
		cur = this.next.due
	}

	next := &this.next
	old := this.entries[key]
	if old != nil {
		if old.due.Equal(due) {
			return
		}
		if old.next != nil {
			old.next.prev = old.prev
		}
		*old.prev = old.next
		if old.due.Before(due) {
			next = old.prev
		}
	} else {
		old = &scheduleEntry{
			ScheduleEntry: ScheduleEntry{key: key},
		}
	}
	now := time.Now()
	if !due.After(now) {
		this.exec.Execute(key)
		return
	}

	old.due = due
	for *next != nil {
		if (*next).due.After(due) {
			old.next = *next
			old.prev = next
			(*next).prev = &old.next
			*next = old
			break
		}
		next = &(*next).next
	}
	this.entries[key] = old
	if (*next) == nil {
		*next = old
		old.prev = next
		old.next = nil
	}

	if this.next.due != cur {
		this.timer.Next(this.next.due.Sub(now))
	}
}

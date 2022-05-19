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
 * limitations under the License.
 *
 */

package utils

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const PERIOD = 10 * time.Millisecond

type Execution struct {
	d int
	k ScheduleKey
}

func (this Execution) String() string {
	return fmt.Sprintf("%5d: %s", this.d, this.k)
}

type Result struct {
	lock   sync.Mutex
	start  time.Time
	result []Execution
}

func (this *Result) Execute(key ScheduleKey) {
	this.lock.Lock()
	defer this.lock.Unlock()
	d := time.Now().Sub(this.start) + (PERIOD / 2)
	this.result = append(this.result, Execution{int(d / PERIOD), key})
}

func (this *Result) String() string {
	this.lock.Lock()
	defer this.lock.Unlock()
	return fmt.Sprintf("%v", this.result)
}

var _ = Describe("Schedule", func() {
	var sched *Schedule
	var result *Result

	BeforeEach(func() {
		result = &Result{start: time.Now()}
		sched = NewSchedule(context.Background(), result)
	})
	AfterEach(func() {
		sched.Reset()
	})

	It("dummy", func() {
		timer := time.NewTimer(0 * time.Second)
		time.Sleep(PERIOD)
		fmt.Printf("%t\n", timer.Stop())
		timer.Reset(2 * PERIOD)
		<-timer.C
		fmt.Printf("first\n")
		time.Sleep(3 * PERIOD)
		select {
		case <-timer.C:
		default:
			Fail("second not ready")
		}
		select {
		case <-timer.C:
			Fail("oops, got third")
		default:
		}
	})

	Context("queue", func() {
		It("queues one entry", func() {
			sched.ScheduleAfter("A", 2*time.Hour)
			list := sched.List()
			Expect(list).To(Equal([]ScheduleKey{"A"}))
		})

		It("appends entry", func() {
			sched.ScheduleAfter("A", 2*time.Hour)
			sched.ScheduleAfter("B", 4*time.Hour)
			list := sched.List()
			Expect(list).To(Equal([]ScheduleKey{"A", "B"}))
			fmt.Printf("%v\n", sched.ListSchedule())
		})

		It("inserts entry in between", func() {
			sched.ScheduleAfter("A", 2*time.Hour)
			sched.ScheduleAfter("B", 4*time.Hour)
			sched.ScheduleAfter("C", 3*time.Hour)
			list := sched.List()
			Expect(list).To(Equal([]ScheduleKey{"A", "C", "B"}))
		})

		It("prepends entry", func() {
			sched.ScheduleAfter("A", 2*time.Hour)
			sched.ScheduleAfter("B", 1*time.Hour)
			list := sched.List()
			Expect(list).To(Equal([]ScheduleKey{"B", "A"}))
		})

		It("delays entry", func() {
			sched.ScheduleAfter("A", 1*time.Hour)
			sched.ScheduleAfter("B", 2*time.Hour)
			sched.ScheduleAfter("C", 3*time.Hour)
			sched.ScheduleAfter("B", 4*time.Hour)
			list := sched.List()
			Expect(list).To(Equal([]ScheduleKey{"A", "C", "B"}))
		})
		It("keeps order", func() {
			sched.ScheduleAfter("A", 1*time.Hour)
			sched.ScheduleAfter("B", 2*time.Hour)
			sched.ScheduleAfter("C", 4*time.Hour)
			sched.ScheduleAfter("B", 3*time.Hour)
			list := sched.List()
			Expect(list).To(Equal([]ScheduleKey{"A", "B", "C"}))
		})
		It("reinserts", func() {
			sched.ScheduleAfter("A", 2*time.Hour)
			sched.ScheduleAfter("B", 3*time.Hour)
			sched.ScheduleAfter("C", 4*time.Hour)
			sched.ScheduleAfter("B", 1*time.Hour)
			list := sched.List()
			Expect(list).To(Equal([]ScheduleKey{"B", "A", "C"}))
		})
		It("delete first", func() {
			sched.ScheduleAfter("A", 2*time.Hour)
			sched.ScheduleAfter("B", 3*time.Hour)
			sched.ScheduleAfter("C", 4*time.Hour)
			sched.Delete("A")
			list := sched.List()
			Expect(list).To(Equal([]ScheduleKey{"B", "C"}))
		})
		It("delete middle", func() {
			sched.ScheduleAfter("A", 2*time.Hour)
			sched.ScheduleAfter("B", 3*time.Hour)
			sched.ScheduleAfter("C", 4*time.Hour)
			sched.Delete("B")
			list := sched.List()
			Expect(list).To(Equal([]ScheduleKey{"A", "C"}))
		})
		It("delete last", func() {
			sched.ScheduleAfter("A", 2*time.Hour)
			sched.ScheduleAfter("B", 3*time.Hour)
			sched.ScheduleAfter("C", 4*time.Hour)
			sched.Delete("C")
			list := sched.List()
			Expect(list).To(Equal([]ScheduleKey{"A", "B"}))
		})
		It("delete last and append new", func() {
			sched.ScheduleAfter("A", 2*time.Hour)
			sched.ScheduleAfter("B", 3*time.Hour)
			sched.ScheduleAfter("C", 4*time.Hour)
			sched.Delete("C")
			sched.ScheduleAfter("D", 4*time.Hour)
			list := sched.List()
			Expect(list).To(Equal([]ScheduleKey{"A", "B", "D"}))
		})
	})
	Context("exec", func() {
		It("executes order", func() {
			sched.ScheduleAfter("A", 1*PERIOD)
			sched.ScheduleAfter("C", 3*PERIOD)
			sched.ScheduleAfter("B", 2*PERIOD)
			time.Sleep(4 * PERIOD)
			fmt.Printf("RES: %s\n", result)
			Expect(result.result).To(Equal([]Execution{{1, "A"}, {2, "B"}, {3, "C"}}))
		})
		It("restarts", func() {
			sched.ScheduleAfter("A", 1*PERIOD)
			sched.ScheduleAfter("C", 3*PERIOD)
			time.Sleep(4 * PERIOD)
			sched.ScheduleAfter("B", 2*PERIOD)
			time.Sleep(3 * PERIOD)
			fmt.Printf("RES: %s\n", result)
			Expect(result.result).To(Equal([]Execution{{1, "A"}, {3, "C"}, {6, "B"}}))
		})
		It("inserts after sched", func() {
			sched.ScheduleAfter("A", 1*PERIOD)
			sched.ScheduleAfter("C", 4*PERIOD)
			time.Sleep(2 * PERIOD)
			sched.ScheduleAfter("B", 1*PERIOD)
			sched.ScheduleAfter("D", 3*PERIOD)
			time.Sleep(4 * PERIOD)
			fmt.Printf("RES: %s\n", result)
			Expect(result.result).To(Equal([]Execution{{1, "A"}, {3, "B"}, {4, "C"}, {5, "D"}}))
		})
		It("resets for earlier", func() {
			sched.ScheduleAfter("C", 3*PERIOD)
			sched.ScheduleAfter("D", 4*PERIOD)
			sched.ScheduleAfter("B", 2*PERIOD)
			sched.ScheduleAfter("A", 1*PERIOD)
			time.Sleep(5 * PERIOD)
			fmt.Printf("RES: %s\n", result)
			Expect(result.result).To(Equal([]Execution{{1, "A"}, {2, "B"}, {3, "C"}, {4, "D"}}))
		})

		It("reschedule first", func() {
			sched.ScheduleAfter("A", 3*PERIOD)
			sched.ScheduleAfter("B", 4*PERIOD)
			time.Sleep(1 * PERIOD)
			sched.ScheduleAfter("A", 1*PERIOD)
			time.Sleep(4 * PERIOD)
			fmt.Printf("RES: %s\n", result)
			Expect(result.result).To(Equal([]Execution{{2, "A"}, {4, "B"}}))
		})

		It("delete first", func() {
			sched.ScheduleAfter("A", 3*PERIOD)
			sched.ScheduleAfter("B", 4*PERIOD)
			time.Sleep(1 * PERIOD)
			sched.Delete("A")
			time.Sleep(4 * PERIOD)
			fmt.Printf("RES: %s\n", result)
			Expect(result.result).To(Equal([]Execution{{4, "B"}}))
		})
	})
})

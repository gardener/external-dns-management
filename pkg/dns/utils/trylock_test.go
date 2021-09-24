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
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const wait = 20 * time.Microsecond
const tryLocked = 1000000

var _ = Describe("TryLock", func() {
	It("deals correctly with lock/unlock", func() {
		lock := NewTryLock()
		lock.Lock()

		var counter uint32
		go func() {
			atomic.AddUint32(&counter, 1)
			lock.Lock()
			atomic.AddUint32(&counter, 2)
			lock.Unlock()
		}()

		time.Sleep(wait)
		Expect(atomic.LoadUint32(&counter)).To(Equal(uint32(1)))

		lock.Unlock()
		time.Sleep(wait)
		Expect(atomic.LoadUint32(&counter)).To(Equal(uint32(3)))

		first := lock.TryLockSpinning(wait)
		Expect(first).To(BeTrue())
		second := lock.TryLockSpinning(wait)
		Expect(second).To(BeFalse())
		lock.Unlock()
	})

	It("deals correctly with mixture of lock/trylock", func() {
		lock := NewTryLock()
		lock.Lock()
		secondLock := lock.TryLockSpinning(20 * time.Millisecond)
		Expect(secondLock).To(BeFalse())

		counters := make([]uint64, 3)
		tryLocker := func(c *uint64) {
			atomic.StoreUint64(c, 1)
			for {
				if lock.TryLock() {
					atomic.StoreUint64(c, uint64(time.Now().Nanosecond()))
					time.Sleep(10 * wait)
					lock.Unlock()
					return
				}
				time.Sleep(wait)
			}
		}
		for i := 0; i < len(counters); i++ {
			go tryLocker(&counters[i])
		}

		time.Sleep(wait)
		for i := 0; i < len(counters); i++ {
			Expect(atomic.LoadUint64(&counters[i])).To(Equal(uint64(1)))
		}
		lock.Unlock()

		time.Sleep(2 * wait)
		var c2 uint32
		go func() {
			atomic.AddUint32(&c2, 1)
			lock.Lock()
			atomic.AddUint32(&c2, 2)
			lock.Unlock()
		}()

		time.Sleep(20 * wait)
		Expect(atomic.LoadUint32(&c2)).To(Equal(uint32(3)))

		for i := 0; i < len(counters); i++ {
			for j := 0; j < len(counters); j++ {
				if i != j && counters[i] > counters[j] {
					Expect(counters[i]-counters[j] > uint64((10 * wait).Nanoseconds())).To(BeTrue())
				}
			}
		}
	})
})

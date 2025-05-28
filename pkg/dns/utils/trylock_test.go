// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const wait = 50 * time.Microsecond

var _ = Describe("TryLock", func() {
	It("deals correctly with lock/unlock", func() {
		lock := NewTryLock(context.Background())
		err := lock.Lock()
		Expect(err).ToNot(HaveOccurred())

		var counter uint32
		var err2 error
		go func() {
			atomic.AddUint32(&counter, 1)
			err2 = lock.Lock()
			atomic.AddUint32(&counter, 2)
			lock.Unlock()
		}()

		Eventually(func() uint32 {
			time.Sleep(wait)
			return atomic.LoadUint32(&counter)
		}).WithPolling(10 * wait).To(Equal(uint32(1)))

		lock.Unlock()
		Eventually(func() uint32 {
			time.Sleep(wait)
			return atomic.LoadUint32(&counter)
		}).WithPolling(10 * wait).To(Equal(uint32(3)))
		Expect(err2).ToNot(HaveOccurred())

		first := lock.TryLockSpinning(wait)
		Expect(first).To(BeTrue())
		second := lock.TryLockSpinning(wait)
		Expect(second).To(BeFalse())
		lock.Unlock()
	})

	It("deals correctly with mixture of lock/trylock", func() {
		lock := NewTryLock()
		err := lock.Lock()
		Expect(err).ToNot(HaveOccurred())
		secondLock := lock.TryLockSpinning(20 * time.Millisecond)
		Expect(secondLock).To(BeFalse())

		counters := make([]uint64, 3)
		wgTryLockers := &sync.WaitGroup{}
		wgTryLockers.Add(len(counters))
		tryLocker := func(c *uint64) {
			atomic.StoreUint64(c, 1)
			for {
				if lock.TryLock() {
					atomic.StoreUint64(c, uint64(time.Now().Nanosecond()))
					time.Sleep(10 * wait)
					lock.Unlock()
					wgTryLockers.Done()
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

		wg := &sync.WaitGroup{}
		wg.Add(3)
		time.Sleep(wait)
		var err2 error
		go func() {
			wg.Done()
			err2 = lock.Lock()
			wg.Done()
			lock.Unlock()
			wg.Done()
		}()

		wg.Wait()
		Expect(err2).ToNot(HaveOccurred())

		wgTryLockers.Wait()

		for i := 0; i < len(counters); i++ {
			for j := 0; j < len(counters); j++ {
				if i != j && counters[i] > counters[j] {
					Expect(counters[i] - counters[j]).To(BeNumerically(">", uint64((10 * wait).Nanoseconds())))
				}
			}
		}
	})
})

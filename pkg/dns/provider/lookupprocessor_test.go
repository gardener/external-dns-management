// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	ginkgov2 "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/atomic"
)

type testEnqueuer struct {
	enqueuedCount map[resources.ObjectName]int
	stopped       atomic.Bool
}

func (e *testEnqueuer) EnqueueKey(key resources.ClusterObjectKey) error {
	if e.stopped.Load() {
		return nil
	}
	e.enqueuedCount[key.ObjectName()]++
	return nil
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type mockLookupHostResult struct {
	ips []net.IP
	err error
}

type mockLookupHost struct {
	delay       time.Duration
	lookupMap   map[string]mockLookupHostResult
	lock        sync.Mutex
	lookupCount map[string]int
	stopped     atomic.Bool
	retryMap    map[string]int
}

func (lh *mockLookupHost) LookupHost(hostname string) ([]net.IP, error) {
	time.Sleep(lh.delay)
	retry := false
	lh.lock.Lock()
	if !lh.stopped.Load() {
		lh.lookupCount[hostname] += 1
	}
	if lh.retryMap != nil && lh.retryMap[hostname] > 0 {
		retry = true
		lh.retryMap[hostname]--
	}
	lh.lock.Unlock()
	if retry {
		time.Sleep(lh.delay)
		return nil, timeoutError{}
	}
	result, ok := lh.lookupMap[hostname]
	if !ok {
		return nil, fmt.Errorf("host not found")
	}
	return result.ips, result.err
}

type lookupStat struct {
	count       int
	targetCount int
	errorCount  int
	duration    time.Duration
}

type testMetrics struct {
	lock    sync.Mutex
	jobs    int
	lookups map[resources.ObjectName]lookupStat
	stopped atomic.Bool
}

var _ lookupMetrics = &testMetrics{}

func (t *testMetrics) IncrSkipped() {
}

func (t *testMetrics) IncrHostnameLookups(name resources.ObjectName, targetCount, errorCount int, duration time.Duration) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if t.stopped.Load() {
		return
	}
	stat := t.lookups[name]
	stat.count++
	stat.targetCount += targetCount
	stat.errorCount += errorCount
	stat.duration += duration
	t.lookups[name] = stat
}

func (t *testMetrics) ReportCurrentJobCount(count int) {
	t.jobs = count
}

func (t *testMetrics) IncrLookupChanged(_ resources.ObjectName) {
}

func (t *testMetrics) RemoveJob(_ resources.ObjectName) {
}

var _ = ginkgov2.Describe("Lookup processor", func() {
	var (
		processor *lookupProcessor
		enqueuer  *testEnqueuer
		mlh       *mockLookupHost
		metrics   *testMetrics

		nameE1    = resources.NewObjectName("ns1", "e1")
		nameE2    = resources.NewObjectName("ns1", "e2")
		nameE3    = resources.NewObjectName("ns1", "e3")
		ctx       context.Context
		ctxCancel context.CancelFunc

		cancel = func() {
			enqueuer.stopped.Store(true)
			mlh.stopped.Store(true)
			metrics.stopped.Store(true)
			ctxCancel()
			time.Sleep(10 * time.Millisecond)
			Expect(processor.running.Load()).To(BeFalse())
		}
	)

	ginkgov2.BeforeEach(func() {
		enqueuer = &testEnqueuer{enqueuedCount: map[resources.ObjectName]int{}}
		metrics = &testMetrics{lookups: map[resources.ObjectName]lookupStat{}}
		processor = newLookupProcessor(logger.New(), enqueuer, 2, 10*time.Millisecond, "default", metrics)
		mlh = &mockLookupHost{
			delay: 1 * time.Microsecond,
			lookupMap: map[string]mockLookupHostResult{
				"host1":        {ips: []net.IP{net.ParseIP("1.1.1.1")}},
				"host2":        {ips: []net.IP{net.ParseIP("1.1.1.2")}},
				"host3a":       {ips: []net.IP{net.ParseIP("1.1.1.3")}},
				"host3b":       {ips: []net.IP{net.ParseIP("1.1.2.3")}},
				"host3c":       {ips: []net.IP{net.ParseIP("1.1.3.3"), net.ParseIP("1.1.3.4"), net.ParseIP("fc00::3")}},
				"host3c-alias": {ips: []net.IP{net.ParseIP("1.1.3.3"), net.ParseIP("1.1.3.4"), net.ParseIP("fc00::3")}},
			},
			lookupCount: map[string]int{},
		}
		lookupHost.lock.Lock()
		lookupHost.lookupHost = mlh.LookupHost
		lookupHost.lock.Unlock()
		lookupHost.waitLookupRetry = 5 * time.Millisecond
		ctx, ctxCancel = context.WithCancel(context.Background())
	})

	ginkgov2.It("lookupAllHostnamesIPs should return expected results", func() {
		results1 := lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c")
		Expect(results1.ipv4Addrs).To(HaveLen(4))
		Expect(results1.ipv6Addrs).To(HaveLen(1))
		Expect(results1.allIPAddrs).To(HaveLen(5))
		results2 := lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c", "host3c-alias")
		Expect(results2.ipv4Addrs).To(Equal(results1.ipv4Addrs))
		Expect(results2.ipv6Addrs).To(Equal(results1.ipv6Addrs))
		Expect(results2.allIPAddrs).To(Equal(results1.allIPAddrs))
	})

	ginkgov2.It("lookupAllHostnamesIPs should return expected results with retries", func() {
		mlh.retryMap = map[string]int{"host3b": 3}
		results1 := lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c")
		Expect(results1.ipv4Addrs).To(HaveLen(4))
		Expect(results1.ipv6Addrs).To(HaveLen(1))
		Expect(results1.allIPAddrs).To(HaveLen(5))
		results2 := lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c", "host3c-alias")
		Expect(results2.allIPAddrs).To(Equal(results1.allIPAddrs))
	})

	ginkgov2.It("lookupAllHostnamesIPs should return reduced results after too many retries", func() {
		mlh.retryMap = map[string]int{"host3b": lookupHost.maxLookupRetries + 1}
		results1 := lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c")
		Expect(results1.ipv4Addrs).To(HaveLen(3))
		Expect(results1.ipv6Addrs).To(HaveLen(1))
		Expect(results1.allIPAddrs).To(HaveLen(4))
	})

	ginkgov2.It("performs multiple lookup jobs regularly", func() {
		go processor.Run(ctx)
		processor.Upsert(nameE1, lookupAllHostnamesIPs(ctx, "host1"), 1*time.Millisecond)
		processor.Upsert(nameE2, lookupAllHostnamesIPs(ctx, "host2"), 2*time.Millisecond)
		processor.Upsert(nameE3, lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c"), 3*time.Millisecond)
		time.Sleep(processor.checkPeriod)

		time.Sleep(18 * time.Millisecond)
		processor.Delete(nameE3)
		processor.Delete(nameE3)
		time.Sleep(18 * time.Millisecond)
		cancel()

		mlh.lock.Lock()
		count1 := mlh.lookupCount["host1"]
		count2 := mlh.lookupCount["host2"]
		count3a := mlh.lookupCount["host3a"]
		count3c := mlh.lookupCount["host3c"]
		mlh.lock.Unlock()

		expectCountBetween("count1", count1, 18, 54)
		expectCountBetween("count2", count2, 9, 27)
		expectCountBetween("count3a", count3a, 3, 9)
		expectCountBetween("count3c-count3a", count3c-count3a, -1, 1)
		Expect(enqueuer.enqueuedCount).To(BeEmpty())
		expectCountBetween("skipped", int(processor.skipped.Load()), 0, 10)
	})

	ginkgov2.It("performs multiple lookup jobs but skips on overload", func() {
		mlh.delay = 1900 * time.Microsecond
		go processor.Run(ctx)
		processor.Upsert(nameE1, lookupAllHostnamesIPs(ctx, "host1"), 1*time.Millisecond)
		processor.Upsert(nameE3, lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c"), 1*time.Millisecond)
		time.Sleep(processor.checkPeriod)

		time.Sleep(30 * time.Millisecond)
		cancel()

		count1 := mlh.lookupCount["host1"]
		count3a := mlh.lookupCount["host3a"]
		count3c := mlh.lookupCount["host3c"]
		expectCountBetween("count1", count1, 10, 20)
		expectCountBetween("count3a", count3a, 10, 20)
		expectCountBetween("count3c-count3a", count3c-count3a, -1, 1)
		Expect(enqueuer.enqueuedCount).To(BeEmpty())
		expectCountBetween("skipped", int(processor.skipped.Load()), 20, 50)
	})

	ginkgov2.It("performs multiple lookup jobs and enqueues keys on lookup changes", func() {
		changedIP := net.ParseIP("1.1.1.42")
		go processor.Run(ctx)
		processor.Upsert(nameE1, lookupAllHostnamesIPs(ctx, "host1"), 1*time.Millisecond)
		processor.Upsert(nameE2, lookupAllHostnamesIPs(ctx, "host2"), 1*time.Millisecond)
		processor.Upsert(nameE3, lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c"), 1*time.Millisecond)
		time.Sleep(processor.checkPeriod)

		time.Sleep(10 * time.Millisecond)
		processor.Upsert(nameE3, lookupAllHostnamesIPs(ctx, "host3a", "host3b", "not-existing-host"), 1*time.Millisecond)
		mlh.lock.Lock()
		mlh.lookupMap["host2"].ips[0] = changedIP
		mlh.lock.Unlock()
		time.Sleep(20 * time.Millisecond)
		cancel()

		mlh.lock.Lock()
		count1 := mlh.lookupCount["host1"]
		count2 := mlh.lookupCount["host2"]
		count3a := mlh.lookupCount["host3a"]
		count3c := mlh.lookupCount["host3c"]
		mlh.lock.Unlock()
		expectCountBetween("count1", count1, 20, 40)
		expectCountBetween("count2", count2, 20, 40)
		expectCountBetween("count3a", count3a, 20, 40)
		expectCountBetween("count3c", count3c, 5, 15)
		Expect(enqueuer.enqueuedCount).To(HaveLen(2))
		Expect(enqueuer.enqueuedCount[nameE2]).To(Equal(1))
		Expect(enqueuer.enqueuedCount[nameE3]).To(Equal(1))
		expectCountBetween("skipped", int(processor.skipped.Load()), 0, 10)
		metrics.lock.Lock()
		stat1 := metrics.lookups[nameE1]
		metrics.lock.Unlock()
		expectCountBetween("stat1.count-count1", stat1.count-count1, -1, 1)
		metrics.lock.Lock()
		stat3 := metrics.lookups[nameE3]
		metrics.lock.Unlock()
		expectCountBetween("stat3.count-count3a", stat3.count-count3a, -1, 1)
		Expect(stat3.targetCount).To(Equal(count3a * 3))
		expectCountBetween("count not-existing-host", stat3.errorCount, 10, 30)
	})
})

func expectCountBetween(name string, actual, lower, upper int) {
	Expect(actual >= lower && actual <= upper).To(BeTrue(), fmt.Sprintf("%d <= %s(%d) <= %d", lower, name, actual, upper))
}

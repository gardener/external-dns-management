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
)

type testEnqueuer struct {
	enqueuedCount map[resources.ObjectName]int
}

func (e *testEnqueuer) EnqueueKey(key resources.ClusterObjectKey) error {
	e.enqueuedCount[key.ObjectName()]++
	return nil
}

type mockLookupHostResult struct {
	ips []net.IP
	err error
}

type mockLookupHost struct {
	delay       time.Duration
	lookupMap   map[string]mockLookupHostResult
	lock        sync.Mutex
	lookupCount map[string]int
}

func (lh *mockLookupHost) LookupHost(hostname string) ([]net.IP, error) {
	time.Sleep(lh.delay)
	lh.lock.Lock()
	lh.lookupCount[hostname] += 1
	lh.lock.Unlock()
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
}

var _ lookupMetrics = &testMetrics{}

func (t *testMetrics) IncrSkipped() {
}

func (t *testMetrics) IncrHostnameLookups(name resources.ObjectName, targetCount, errorCount int, duration time.Duration) {
	t.lock.Lock()
	defer t.lock.Unlock()
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

		nameE1 = resources.NewObjectName("ns1", "e1")
		nameE2 = resources.NewObjectName("ns1", "e2")
		nameE3 = resources.NewObjectName("ns1", "e3")
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
		lookupHost = mlh.LookupHost
	})

	ginkgov2.It("lookupAllHostnamesIPs should return expected results", func() {
		ctx := context.Background()
		results1 := lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c")
		Expect(results1.ipv4Addrs).To(HaveLen(4))
		Expect(results1.ipv6Addrs).To(HaveLen(1))
		Expect(results1.allIPAddrs).To(HaveLen(5))
		results2 := lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c", "host3c-alias")
		Expect(results2.allIPAddrs).To(Equal(results1.allIPAddrs))
	})

	ginkgov2.It("performs multiple lookup jobs regularly", func() {
		ctx, cancel := context.WithCancel(context.Background())
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
		time.Sleep(1 * time.Millisecond)
		Expect(processor.running.Load()).To(BeFalse())

		count1 := mlh.lookupCount["host1"]
		count2 := mlh.lookupCount["host2"]
		count3a := mlh.lookupCount["host3a"]
		count3c := mlh.lookupCount["host3c"]
		expectCountBetween("count1", count1, 18, 54)
		expectCountBetween("count2", count2, 9, 27)
		expectCountBetween("count3a", count3a, 3, 9)
		Expect(count3c).To(Equal(count3a))
		Expect(enqueuer.enqueuedCount).To(BeEmpty())
		expectCountBetween("skipped", int(processor.skipped.Load()), -1, 6)
	})

	ginkgov2.It("performs multiple lookup jobs but skips on overload", func() {
		ctx, cancel := context.WithCancel(context.Background())
		mlh.delay = 1900 * time.Microsecond
		go processor.Run(ctx)

		processor.Upsert(nameE1, lookupAllHostnamesIPs(ctx, "host1"), 1*time.Millisecond)
		processor.Upsert(nameE3, lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c"), 1*time.Millisecond)
		time.Sleep(processor.checkPeriod)

		time.Sleep(30 * time.Millisecond)
		cancel()
		time.Sleep(1 * time.Millisecond)
		Expect(processor.running.Load()).To(BeFalse())

		count1 := mlh.lookupCount["host1"]
		count3a := mlh.lookupCount["host3a"]
		count3c := mlh.lookupCount["host3c"]
		expectCountBetween("count1", count1, 10, 20)
		expectCountBetween("count3a", count3a, 10, 20)
		Expect(count3c).To(Equal(count3a))
		Expect(enqueuer.enqueuedCount).To(BeEmpty())
		expectCountBetween("skipped", int(processor.skipped.Load()), 20, 50)
	})

	ginkgov2.It("performs multiple lookup jobs and enqueues keys on lookup changes", func() {
		changedIP := net.ParseIP("1.1.1.42")
		ctx, cancel := context.WithCancel(context.Background())
		go processor.Run(ctx)

		processor.Upsert(nameE1, lookupAllHostnamesIPs(ctx, "host1"), 1*time.Millisecond)
		processor.Upsert(nameE2, lookupAllHostnamesIPs(ctx, "host2"), 1*time.Millisecond)
		processor.Upsert(nameE3, lookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c"), 1*time.Millisecond)
		time.Sleep(processor.checkPeriod)

		time.Sleep(10 * time.Millisecond)
		processor.Upsert(nameE3, lookupAllHostnamesIPs(ctx, "host3a", "host3b", "not-existing-host"), 1*time.Millisecond)
		mlh.lookupMap["host2"].ips[0] = changedIP
		time.Sleep(20 * time.Millisecond)
		cancel()
		time.Sleep(1 * time.Millisecond)
		Expect(processor.running.Load()).To(BeFalse())

		count1 := mlh.lookupCount["host1"]
		count2 := mlh.lookupCount["host2"]
		count3a := mlh.lookupCount["host3a"]
		count3c := mlh.lookupCount["host3c"]
		expectCountBetween("count1", count1, 20, 40)
		expectCountBetween("count2", count2, 20, 40)
		expectCountBetween("count3a", count3a, 20, 40)
		expectCountBetween("count3c", count3c, 5, 15)
		Expect(enqueuer.enqueuedCount).To(HaveLen(2))
		Expect(enqueuer.enqueuedCount[nameE2]).To(Equal(1))
		Expect(enqueuer.enqueuedCount[nameE3]).To(Equal(1))
		expectCountBetween("skipped", int(processor.skipped.Load()), -1, 6)
		stat1 := metrics.lookups[nameE1]
		Expect(stat1.count).To(Equal(count1))
		stat3 := metrics.lookups[nameE3]
		Expect(stat3.count).To(Equal(count3a))
		Expect(stat3.targetCount).To(Equal(count3a * 3))
		expectCountBetween("count not-existing-host", stat3.errorCount, 10, 30)
	})
})

func expectCountBetween(name string, actual, lower, upper int) {
	Expect(actual > lower && actual < upper).To(BeTrue(), fmt.Sprintf("%d < %s(%d) < %d", lower, name, actual, upper))
}

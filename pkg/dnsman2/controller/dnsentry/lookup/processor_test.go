// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lookup

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	ginkgov2 "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/atomic"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const quantum = 5 * time.Millisecond

type testTrigger struct {
	triggerCount map[client.ObjectKey]int
	stopped      atomic.Bool
}

func (t *testTrigger) TriggerReconciliation(_ context.Context, key client.ObjectKey) error {
	if t.stopped.Load() {
		return nil
	}
	t.triggerCount[key]++
	return nil
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
	lookups map[client.ObjectKey]lookupStat
	stopped atomic.Bool
}

var _ LookupMetrics = &testMetrics{}

func (t *testMetrics) IncrSkipped() {
}

func (t *testMetrics) IncrHostnameLookups(name client.ObjectKey, targetCount, errorCount int, duration time.Duration) {
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

func (t *testMetrics) IncrLookupChanged(_ client.ObjectKey) {
}

func (t *testMetrics) RemoveJob(_ client.ObjectKey) {
}

var _ = ginkgov2.Describe("Lookup processor", func() {
	var (
		processor    *lookupProcessor
		entryTrigger *testTrigger
		mlh          *MockLookupHost
		metrics      *testMetrics

		nameE1    = client.ObjectKey{Namespace: "ns1", Name: "e1"}
		nameE2    = client.ObjectKey{Namespace: "ns1", Name: "e2"}
		nameE3    = client.ObjectKey{Namespace: "ns1", Name: "e3"}
		ctx       context.Context
		ctxCancel context.CancelFunc

		cancel = func() {
			entryTrigger.stopped.Store(true)
			mlh.stopped.Store(true)
			metrics.stopped.Store(true)
			ctxCancel()
			time.Sleep(10 * quantum)
			Expect(processor.running.Load()).To(BeFalse())
		}
	)

	ginkgov2.BeforeEach(func() {
		entryTrigger = &testTrigger{triggerCount: map[client.ObjectKey]int{}}
		processor = NewLookupProcessor(log, entryTrigger, 2, 10*quantum)
		metrics = &testMetrics{lookups: map[client.ObjectKey]lookupStat{}}
		processor.metrics = metrics
		mlh = &MockLookupHost{
			delay: 1 * time.Microsecond,
			lookupMap: map[string]MockLookupHostResult{
				"host1":        {IPs: []net.IP{net.ParseIP("1.1.1.1")}},
				"host2":        {IPs: []net.IP{net.ParseIP("1.1.1.2")}},
				"host3a":       {IPs: []net.IP{net.ParseIP("1.1.1.3")}},
				"host3b":       {IPs: []net.IP{net.ParseIP("1.1.2.3")}},
				"host3c":       {IPs: []net.IP{net.ParseIP("1.1.3.3"), net.ParseIP("1.1.3.4"), net.ParseIP("fc00::3")}},
				"host3c-alias": {IPs: []net.IP{net.ParseIP("1.1.3.3"), net.ParseIP("1.1.3.4"), net.ParseIP("fc00::3")}},
			},
			lookupCount: map[string]int{},
		}
		SetLookupFunc(mlh.LookupHost)
		lookupHost.waitLookupRetry = 5 * quantum
		ctx, ctxCancel = context.WithCancel(context.Background())
	})

	ginkgov2.It("LookupAllHostnamesIPs should return expected results", func() {
		results1 := LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c")
		Expect(results1.IPv4Addrs).To(HaveLen(4))
		Expect(results1.IPv6Addrs).To(HaveLen(1))
		Expect(results1.AllIPAddrs).To(HaveLen(5))
		results2 := LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c", "host3c-alias")
		Expect(results2.IPv4Addrs).To(Equal(results1.IPv4Addrs))
		Expect(results2.IPv6Addrs).To(Equal(results1.IPv6Addrs))
		Expect(results2.AllIPAddrs).To(Equal(results1.AllIPAddrs))
	})

	ginkgov2.It("LookupAllHostnamesIPs should return expected results with retries", func() {
		mlh.retryMap = map[string]int{"host3b": 3}
		results1 := LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c")
		Expect(results1.IPv4Addrs).To(HaveLen(4))
		Expect(results1.IPv6Addrs).To(HaveLen(1))
		Expect(results1.AllIPAddrs).To(HaveLen(5))
		results2 := LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c", "host3c-alias")
		Expect(results2.AllIPAddrs).To(Equal(results1.AllIPAddrs))
	})

	ginkgov2.It("LookupAllHostnamesIPs should return reduced results with persisting timeouts", func() {
		mlh.retryMap = map[string]int{"host3b": 10}
		results1 := LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c")
		Expect(results1.IPv4Addrs).To(HaveLen(3))
		Expect(results1.IPv6Addrs).To(HaveLen(1))
		Expect(results1.AllIPAddrs).To(HaveLen(4))
		Expect(results1.HasTimeoutError()).To(BeTrue())
		Expect(results1.HasOnlyNotFoundError()).To(BeFalse())
	})

	ginkgov2.It("LookupAllHostnamesIPs should return expected results with temporary server failures", func() {
		mlh.serverFailureMap = map[string]int{"host3b": 3}
		results1 := LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c")
		Expect(results1.IPv4Addrs).To(HaveLen(4))
		Expect(results1.IPv6Addrs).To(HaveLen(1))
		Expect(results1.AllIPAddrs).To(HaveLen(5))
		results2 := LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c", "host3c-alias")
		Expect(results2.AllIPAddrs).To(Equal(results1.AllIPAddrs))
	})

	ginkgov2.It("LookupAllHostnamesIPs should return reduced results with persisting temporary server errors", func() {
		mlh.serverFailureMap = map[string]int{"host3b": 10}
		results1 := LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c")
		Expect(results1.IPv4Addrs).To(HaveLen(3))
		Expect(results1.IPv6Addrs).To(HaveLen(1))
		Expect(results1.AllIPAddrs).To(HaveLen(4))
		Expect(results1.HasTimeoutError()).To(BeTrue())
		Expect(results1.HasOnlyNotFoundError()).To(BeFalse())
	})

	ginkgov2.It("LookupAllHostnamesIPs should return reduced results with persisting temporary server errors", func() {
		results1 := LookupAllHostnamesIPs(ctx, "host3a", "host3b-not-existing", "host3c")
		Expect(results1.IPv4Addrs).To(HaveLen(3))
		Expect(results1.IPv6Addrs).To(HaveLen(1))
		Expect(results1.AllIPAddrs).To(HaveLen(4))
		Expect(results1.HasTimeoutError()).To(BeFalse())
		Expect(results1.HasOnlyNotFoundError()).To(BeTrue())
	})

	ginkgov2.It("LookupAllHostnamesIPs should return reduced results after too many retries", func() {
		mlh.retryMap = map[string]int{"host3b": lookupHost.maxLookupRetries + 1}
		results1 := LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c")
		Expect(results1.IPv4Addrs).To(HaveLen(3))
		Expect(results1.IPv6Addrs).To(HaveLen(1))
		Expect(results1.AllIPAddrs).To(HaveLen(4))
	})

	ginkgov2.It("performs multiple lookup jobs regularly", func() {
		go processor.Run(ctx)
		processor.Upsert(ctx, nameE1, LookupAllHostnamesIPs(ctx, "host1"), 1*quantum)
		processor.Upsert(ctx, nameE2, LookupAllHostnamesIPs(ctx, "host2"), 2*quantum)
		processor.Upsert(ctx, nameE3, LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c"), 3*quantum)
		time.Sleep(processor.checkPeriod)

		time.Sleep(18 * quantum)
		processor.Delete(nameE3)
		processor.Delete(nameE3)
		time.Sleep(18 * quantum)
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
		Expect(entryTrigger.triggerCount).To(BeEmpty())
		expectCountBetween("skipped", int(processor.skipped.Load()), 0, 10)
	})

	ginkgov2.It("performs multiple lookup jobs but skips on overload", func() {
		mlh.delay = 19 * quantum / 10
		go processor.Run(ctx)
		processor.Upsert(ctx, nameE1, LookupAllHostnamesIPs(ctx, "host1"), 1*quantum)
		processor.Upsert(ctx, nameE3, LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c"), 1*quantum)
		time.Sleep(processor.checkPeriod)

		time.Sleep(30 * quantum)
		cancel()

		mlh.lock.Lock()
		count1 := mlh.lookupCount["host1"]
		count3a := mlh.lookupCount["host3a"]
		count3c := mlh.lookupCount["host3c"]
		mlh.lock.Unlock()
		expectCountBetween("count1", count1, 10, 20)
		expectCountBetween("count3a", count3a, 10, 20)
		expectCountBetween("count3c-count3a", count3c-count3a, -1, 1)
		Expect(entryTrigger.triggerCount).To(BeEmpty())
		expectCountBetween("skipped", int(processor.skipped.Load()), 20, 50)
	})

	ginkgov2.It("performs multiple lookup jobs and enqueues keys on lookup changes", func() {
		changedIP := net.ParseIP("1.1.1.42")
		go processor.Run(ctx)
		processor.Upsert(ctx, nameE1, LookupAllHostnamesIPs(ctx, "host1"), 1*quantum)
		processor.Upsert(ctx, nameE2, LookupAllHostnamesIPs(ctx, "host2"), 1*quantum)
		processor.Upsert(ctx, nameE3, LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c"), 1*quantum)
		time.Sleep(processor.checkPeriod)

		time.Sleep(10 * quantum)
		processor.Upsert(ctx, nameE3, LookupAllHostnamesIPs(ctx, "host3a", "host3b", "not-existing-host"), 1*quantum)
		mlh.lock.Lock()
		mlh.lookupMap["host2"] = MockLookupHostResult{
			IPs: []net.IP{changedIP},
		}
		mlh.lock.Unlock()
		time.Sleep(20 * quantum)
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
		Expect(entryTrigger.triggerCount).To(HaveLen(2))
		Expect(entryTrigger.triggerCount[nameE2]).To(Equal(1))
		Expect(entryTrigger.triggerCount[nameE3]).To(Equal(1))
		expectCountBetween("skipped", int(processor.skipped.Load()), 0, 10)
		metrics.lock.Lock()
		stat1 := metrics.lookups[nameE1]
		metrics.lock.Unlock()
		expectCountBetween("stat1.count-count1", stat1.count-count1, -1, 1)
		metrics.lock.Lock()
		stat3 := metrics.lookups[nameE3]
		metrics.lock.Unlock()
		expectCountBetween("stat3.count-count3a", stat3.count-count3a, -1, 1)
		Expect(stat3.targetCount).To(Equal(stat3.count * 3))
		expectCountBetween("count not-existing-host", stat3.errorCount, 10, 30)
	})
})

func expectCountBetween(name string, actual, lower, upper int) {
	Expect(actual >= lower && actual <= upper).To(BeTrue(), fmt.Sprintf("%d <= %s(%d) <= %d", lower, name, actual, upper))
}

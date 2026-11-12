// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package lookup

import (
	"context"
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
			Eventually(processor.running.Load, 5*time.Second, quantum).Should(BeFalse())
		}

		startProcessor = func() {
			go func() {
				_ = processor.Start(ctx)
			}()
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
		startProcessor()
		processor.Upsert(ctx, nameE1, LookupAllHostnamesIPs(ctx, "host1"), 1*quantum)
		processor.Upsert(ctx, nameE2, LookupAllHostnamesIPs(ctx, "host2"), 2*quantum)
		processor.Upsert(ctx, nameE3, LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c"), 3*quantum)

		// Wait until host1 has run at least 6 times, then delete e3 and wait for 6 more.
		// This makes progress-gating explicit rather than relying on wall-clock sleeps.
		Eventually(func() int { mlh.lock.Lock(); defer mlh.lock.Unlock(); return mlh.lookupCount["host1"] },
			5*time.Second, quantum).Should(BeNumerically(">=", 6))
		processor.Delete(nameE3)
		processor.Delete(nameE3)
		count3aBeforeDelete := func() int { mlh.lock.Lock(); defer mlh.lock.Unlock(); return mlh.lookupCount["host3a"] }()
		Eventually(func() int { mlh.lock.Lock(); defer mlh.lock.Unlock(); return mlh.lookupCount["host1"] },
			5*time.Second, quantum).Should(BeNumerically(">=", count3aBeforeDelete*6+6))
		cancel()

		mlh.lock.Lock()
		count1 := mlh.lookupCount["host1"]
		count2 := mlh.lookupCount["host2"]
		count3a := mlh.lookupCount["host3a"]
		count3c := mlh.lookupCount["host3c"]
		mlh.lock.Unlock()

		// host1 (1×quantum) fires ~2x host2 (2×quantum) and ~3x host3 (3×quantum)
		Expect(count1).To(BeNumerically(">=", count2))
		Expect(count2).To(BeNumerically(">=", count3a))
		// host3a, host3b, host3c are looked up together, so counts must match
		Expect(count3c - count3a).To(BeNumerically("~", 0, 1))
		// after Delete, host3 must not have run again (allow 1 in-flight)
		Expect(count3a).To(BeNumerically("<=", count3aBeforeDelete+1))
		Expect(entryTrigger.triggerCount).To(BeEmpty())
	})

	ginkgov2.It("performs multiple lookup jobs but skips on overload", func() {
		mlh.delay = 19 * quantum / 10
		startProcessor()
		processor.Upsert(ctx, nameE1, LookupAllHostnamesIPs(ctx, "host1"), 1*quantum)
		processor.Upsert(ctx, nameE3, LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c"), 1*quantum)

		// Wait until the processor has skipped at least 10 jobs — that proves overload detection works.
		Eventually(func() int64 { return processor.skipped.Load() }, 5*time.Second, quantum).
			Should(BeNumerically(">=", 10))
		cancel()

		mlh.lock.Lock()
		count3c := mlh.lookupCount["host3c"]
		count3a := mlh.lookupCount["host3a"]
		mlh.lock.Unlock()
		Expect(count3c - count3a).To(BeNumerically("~", 0, 1))
		Expect(entryTrigger.triggerCount).To(BeEmpty())
	})

	ginkgov2.It("performs multiple lookup jobs and enqueues keys on lookup changes", func() {
		changedIP := net.ParseIP("1.1.1.42")
		startProcessor()
		processor.Upsert(ctx, nameE1, LookupAllHostnamesIPs(ctx, "host1"), 1*quantum)
		processor.Upsert(ctx, nameE2, LookupAllHostnamesIPs(ctx, "host2"), 1*quantum)
		processor.Upsert(ctx, nameE3, LookupAllHostnamesIPs(ctx, "host3a", "host3b", "host3c"), 1*quantum)
		time.Sleep(processor.checkPeriod)

		time.Sleep(10 * quantum)
		// Snapshot host3c count before changing e3's hostnames: after the Upsert, host3c is no longer looked up.
		mlh.lock.Lock()
		count3cSnapshot := mlh.lookupCount["host3c"]
		count3aSnapshot := mlh.lookupCount["host3a"]
		mlh.lock.Unlock()

		processor.Upsert(ctx, nameE3, LookupAllHostnamesIPs(ctx, "host3a", "host3b", "not-existing-host"), 1*quantum)
		mlh.lock.Lock()
		mlh.lookupMap["host2"] = MockLookupHostResult{
			IPs: []net.IP{changedIP},
		}
		mlh.lock.Unlock()
		// Wait for both triggers to fire, then let in-flight lookups finish before cancelling.
		Eventually(func() int { return len(entryTrigger.triggerCount) }, 5*time.Second, quantum).Should(Equal(2))
		// Sleep one extra cycle so any in-flight goroutine can update oldLookupResults,
		// preventing a spurious re-trigger after the expected state is reached.
		time.Sleep(5 * quantum)
		cancel()

		Expect(entryTrigger.triggerCount).To(HaveLen(2))
		Expect(entryTrigger.triggerCount[nameE2]).To(Equal(1))
		Expect(entryTrigger.triggerCount[nameE3]).To(Equal(1))

		mlh.lock.Lock()
		count1 := mlh.lookupCount["host1"]
		count3c := mlh.lookupCount["host3c"]
		mlh.lock.Unlock()
		// host3a and host3c were looked up together before the Upsert; snapshot counts should match
		Expect(count3cSnapshot - count3aSnapshot).To(BeNumerically("~", 0, 1))
		// host3c must not have been looked up after the Upsert (allow 1 in-flight)
		Expect(count3c - count3cSnapshot).To(BeNumerically("<=", 1))

		metrics.lock.Lock()
		stat1 := metrics.lookups[nameE1]
		metrics.lock.Unlock()
		// metrics count must track lookup count (allow 1 in-flight difference)
		Expect(stat1.count - count1).To(BeNumerically("~", 0, 1))
		metrics.lock.Lock()
		stat3 := metrics.lookups[nameE3]
		metrics.lock.Unlock()
		// stat3.count includes both periodic runs and the Upsert call itself
		Expect(stat3.targetCount).To(Equal(stat3.count * 3))
		// after the Upsert replacing host3c with not-existing-host, each periodic run produces one error
		Expect(stat3.errorCount).To(BeNumerically(">", 0))
	})
})

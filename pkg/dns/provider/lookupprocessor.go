// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"container/heap"
	"context"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"go.uber.org/atomic"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/external-dns-management/pkg/server/metrics"
)

type lookupHostConfig struct {
	lock                       sync.Mutex
	lookupHost                 func(string) ([]net.IP, error)
	maxConcurrentLookupsPerJob int
	maxLookupRetries           int
	waitLookupRetry            time.Duration
}

func defaultLookupHostConfig() lookupHostConfig {
	return lookupHostConfig{
		lookupHost:                 net.LookupIP,
		maxConcurrentLookupsPerJob: 4,
		maxLookupRetries:           5,
		waitLookupRetry:            500 * time.Millisecond,
	}
}

// lookupHost allows to override the default lookup function for testing purposes
var lookupHost lookupHostConfig = defaultLookupHostConfig()

type lookupJob struct {
	objectName resources.ObjectName

	lock             sync.Mutex
	oldLookupResults lookupAllResults
	scheduledAt      time.Time
	interval         time.Duration

	running atomic.Bool
}

func (j *lookupJob) updateWithLock(newResults lookupAllResults, interval time.Duration) bool {
	j.lock.Lock()
	defer j.lock.Unlock()
	j.interval = interval
	j.scheduledAt = time.Now().Add(interval)
	return j.updateLookupResult(newResults)
}

// update updates lookup results and returns if resolved IP addresses have changed.
func (j *lookupJob) updateLookupResult(newResults lookupAllResults) bool {
	changed := !j.oldLookupResults.allIPAddrs.Equal(newResults.allIPAddrs)
	j.oldLookupResults = newResults
	return changed && !newResults.HasTemporaryError()
}

type lookupQueue []*lookupJob

var _ heap.Interface = &lookupQueue{}

func (q lookupQueue) Len() int {
	return len(q)
}

func (q lookupQueue) Less(i, j int) bool {
	return q[i].scheduledAt.Before(q[j].scheduledAt)
}

func (q lookupQueue) Swap(i, j int) {
	tmp := q[i]
	q[i] = q[j]
	q[j] = tmp
}

func (q *lookupQueue) Push(x any) {
	job := x.(*lookupJob)
	*q = append(*q, job)
}

func (q *lookupQueue) Pop() any {
	old := *q
	n := len(old)
	x := old[n-1]
	*q = old[0 : n-1]
	return x
}

type enqueuer interface {
	EnqueueKey(key resources.ClusterObjectKey) error
}

type lookupMetrics interface {
	IncrSkipped()
	IncrHostnameLookups(name resources.ObjectName, hosts, errorCount int, duration time.Duration)
	ReportCurrentJobCount(count int)
	IncrLookupChanged(name resources.ObjectName)
	RemoveJob(name resources.ObjectName)
}

type defaultLookupMetrics struct{}

var _ lookupMetrics = &defaultLookupMetrics{}

func (d defaultLookupMetrics) IncrSkipped() {
	metrics.ReportLookupProcessorIncrSkipped()
}

func (d defaultLookupMetrics) IncrHostnameLookups(name resources.ObjectName, hosts, errorCount int, duration time.Duration) {
	metrics.ReportLookupProcessorIncrHostnameLookups(name, hosts, errorCount, duration)
}

func (d defaultLookupMetrics) ReportCurrentJobCount(count int) {
	metrics.ReportLookupProcessorJobs(count)
}

func (d defaultLookupMetrics) IncrLookupChanged(name resources.ObjectName) {
	metrics.ReportLookupProcessorIncrLookupChanged(name)
}

func (d defaultLookupMetrics) RemoveJob(name resources.ObjectName) {
	metrics.ReportRemovedJob(name)
}

type lookupProcessor struct {
	lock           sync.Mutex
	logger         logger.LogContext
	checkPeriod    time.Duration
	concurrentJobs int
	slots          chan struct{}
	queue          lookupQueue
	cluster        string
	enqueuer       enqueuer
	running        atomic.Bool
	skipped        atomic.Int64
	metrics        lookupMetrics
}

func newLookupProcessor(
	logger logger.LogContext,
	enqueuer enqueuer,
	concurrentJobs int,
	checkPeriod time.Duration,
	cluster string,
	metrics lookupMetrics,
) *lookupProcessor {
	return &lookupProcessor{
		logger:         logger,
		checkPeriod:    checkPeriod,
		concurrentJobs: concurrentJobs,
		slots:          make(chan struct{}, concurrentJobs),
		queue:          lookupQueue{},
		cluster:        cluster,
		enqueuer:       enqueuer,
		metrics:        metrics,
	}
}

// Upsert inserts or updates a lookup job for the given object name.
func (p *lookupProcessor) Upsert(name resources.ObjectName, results lookupAllResults, interval time.Duration) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.incrHostnameLookups(name, results)

	idx := p.find(name)
	if idx >= 0 {
		changed := p.queue[idx].updateWithLock(results, interval)
		heap.Fix(&p.queue, idx)
		if changed {
			p.enqueueKey(name)
		}
		return
	}

	job := &lookupJob{
		scheduledAt:      time.Now().Add(interval),
		objectName:       name,
		interval:         interval,
		oldLookupResults: results,
	}
	heap.Push(&p.queue, job)
	p.metrics.ReportCurrentJobCount(len(p.queue))
}

// Delete removes the lookup job for the given object name.
func (p *lookupProcessor) Delete(name resources.ObjectName) {
	p.lock.Lock()
	defer p.lock.Unlock()

	idx := p.find(name)
	if idx == -1 {
		return
	}
	heap.Remove(&p.queue, idx)
	p.metrics.RemoveJob(name)
	p.metrics.ReportCurrentJobCount(len(p.queue))
}

func (p *lookupProcessor) Run(ctx context.Context) {
	p.running.Store(true)
	defer p.running.Store(false)
	p.logger.Infof("starting lookup processor with %d slots", p.concurrentJobs)

	nextCheck := p.checkPeriod
	for {
		if err := sleep(ctx, nextCheck); err != nil {
			p.logger.Infof("lookup processor stopped: %s", ctx.Err())
			return
		}
		nextCheckTime, skipped := p.runJob(ctx)
		if skipped != nil {
			p.logger.Infof("skipped entry %s as lookup not yet finished", *skipped)
		}
		nextCheck = time.Until(nextCheckTime)
	}
}

func (p *lookupProcessor) runJob(ctx context.Context) (time.Time, *resources.ObjectName) {
	var (
		nextCheck time.Time
		job       *lookupJob
	)
	p.lock.Lock()
	if len(p.queue) == 0 {
		nextCheck = time.Now().Add(p.checkPeriod)
	} else {
		idle := time.Until(p.queue[0].scheduledAt)
		if idle < 0 {
			job = p.queue[0]
			job.scheduledAt = time.Now().Add(job.interval)
			heap.Fix(&p.queue, 0)
		}
		nextCheck = p.queue[0].scheduledAt
	}
	p.lock.Unlock()

	if job != nil {
		if !job.running.CompareAndSwap(false, true) {
			p.skipped.Inc()
			p.metrics.IncrSkipped()
			return nextCheck, &job.objectName
		}
		select {
		case <-ctx.Done():
			job.running.Store(false)
			p.logger.Warnf("lookup cancelled: %s", ctx.Err())
			return nextCheck, &job.objectName
		case p.slots <- struct{}{}: // Acquire semaphore slot
			go func(j *lookupJob) {
				defer func() {
					j.running.Store(false)
					<-p.slots // Release semaphore slot
				}()
				j.lock.Lock()
				defer j.lock.Unlock()
				newLookupResult := lookupAllHostnamesIPs(ctx, j.oldLookupResults.hostnames...)
				p.incrHostnameLookups(j.objectName, newLookupResult)
				if j.updateLookupResult(newLookupResult) {
					p.enqueueKey(j.objectName)
				}
			}(job)
		}
	}
	return nextCheck, nil
}

func (p *lookupProcessor) incrHostnameLookups(name resources.ObjectName, results lookupAllResults) {
	p.metrics.IncrHostnameLookups(name, len(results.hostnames), len(results.errs), results.duration)
}

func (p *lookupProcessor) enqueueKey(name resources.ObjectName) {
	key := resources.NewClusterKey(p.cluster, entryGroupKind, name.Namespace(), name.Name())
	if err := p.enqueuer.EnqueueKey(key); err != nil {
		p.logger.Warnf("failed to enqueue key %s: %v", key, err)
	}
	p.metrics.IncrLookupChanged(name)
}

func (p *lookupProcessor) find(name resources.ObjectName) int {
	for i, job := range p.queue {
		if job.objectName == name {
			return i
		}
	}
	return -1
}

type lookupAllResults struct {
	hostnames  []string
	ipv4Addrs  []string
	ipv6Addrs  []string
	errs       []error
	allIPAddrs sets.Set[string]
	duration   time.Duration
}

// HasTemporaryError returns true if any of the lookup results has a temporary error.
func (r lookupAllResults) HasTemporaryError() bool {
	for _, err := range r.errs {
		if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
			return true
		}
	}
	return false
}

// HasTimeoutError returns true if any of the lookup results has a timeout error.
func (r lookupAllResults) HasTimeoutError() bool {
	for _, err := range r.errs {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return true
		}
	}
	return false
}

// HasOnlyNotFoundError returns true if all lookup results are not found errors.
func (r lookupAllResults) HasOnlyNotFoundError() bool {
	if len(r.errs) == 0 {
		return false
	}
	for _, err := range r.errs {
		if dnsErr, ok := err.(*net.DNSError); !ok || !dnsErr.IsNotFound {
			return false
		}
	}
	return true
}

func lookupAllHostnamesIPs(ctx context.Context, hostnames ...string) lookupAllResults {
	start := time.Now()
	results := make(chan lookupIPsResult, lookupHost.maxConcurrentLookupsPerJob)
	go func() {
		sem := make(chan struct{}, lookupHost.maxConcurrentLookupsPerJob)
		for _, hostname := range hostnames {
			select {
			case <-ctx.Done():
				results <- lookupIPsResult{err: ctx.Err()}
			case sem <- struct{}{}: // Acquire semaphore slot
				go func(h string) {
					defer func() {
						<-sem // Release semaphore slot
					}()
					results <- lookupIPs(h)
				}(hostname)
			}
		}
	}()

	all := lookupAllResults{hostnames: hostnames, allIPAddrs: sets.New[string]()}
	for range len(hostnames) {
		result := <-results
		if result.err != nil {
			all.errs = append(all.errs, result.err)
			continue
		}

		for _, addr := range result.ipv4Addrs {
			if all.allIPAddrs.Has(addr) {
				continue
			}
			all.ipv4Addrs = append(all.ipv4Addrs, addr)
			all.allIPAddrs.Insert(addr)
		}
		for _, addr := range result.ipv6Addrs {
			if all.allIPAddrs.Has(addr) {
				continue
			}
			all.ipv6Addrs = append(all.ipv6Addrs, addr)
			all.allIPAddrs.Insert(addr)
		}
	}
	all.duration = time.Since(start)
	sort.Strings(all.ipv4Addrs)
	sort.Strings(all.ipv6Addrs)
	return all
}

type lookupIPsResult struct {
	ipv4Addrs []string
	ipv6Addrs []string
	err       error
}

func lookupIPs(hostname string) lookupIPsResult {
	var (
		ips []net.IP
		err error
	)
	lookupHost.lock.Lock()
	lookupFunc := lookupHost.lookupHost
	lookupHost.lock.Unlock()
	for i := 1; i <= lookupHost.maxLookupRetries; i++ {
		ips, err = lookupFunc(hostname)
		if err == nil || i == lookupHost.maxLookupRetries {
			break
		}
		if netErr, ok := err.(net.Error); !ok || (!netErr.Timeout() && !netErr.Temporary()) {
			break
		}
		time.Sleep(lookupHost.waitLookupRetry)
	}
	if err != nil {
		if _, ok := err.(*net.DNSError); ok {
			return lookupIPsResult{err: err}
		}
		return lookupIPsResult{err: &net.DNSError{
			Err:         err.Error(),
			Name:        hostname,
			IsTemporary: isTemporaryError(err),
			IsTimeout:   isTimeoutError(err),
			IsNotFound:  isNotFoundError(err),
		}}
	}
	ipv4addrs := make([]string, 0, len(ips))
	ipv6addrs := make([]string, 0, len(ips))
	for _, ip := range ips {
		if ip.To4() != nil {
			ipv4addrs = append(ipv4addrs, ip.String())
		} else if ip.To16() != nil {
			ipv6addrs = append(ipv6addrs, ip.String())
		}
	}
	if len(ipv4addrs) == 0 && len(ipv6addrs) == 0 {
		return lookupIPsResult{err: fmt.Errorf("%s has no IPv4/IPv6 address (of %d addresses)", hostname, len(ips))}
	}
	return lookupIPsResult{ipv4Addrs: ipv4addrs, ipv6Addrs: ipv6addrs}
}

func sleep(ctx context.Context, d time.Duration) error {
	if d < 1*time.Microsecond {
		return nil
	} else if d > 30*time.Second {
		d = 30 * time.Second
	}
	t := time.NewTimer(d)
	select {
	case <-ctx.Done():
		t.Stop()
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func isTemporaryError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Temporary()
	}
	return false
}

func isTimeoutError(err error) bool {
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout()
	}
	return false
}

func isNotFoundError(err error) bool {
	if dnsErr, ok := err.(*net.DNSError); ok {
		return dnsErr.IsNotFound
	}
	return false
}

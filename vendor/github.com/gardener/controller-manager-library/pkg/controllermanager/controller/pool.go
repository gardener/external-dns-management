/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/server/healthz"

	"k8s.io/client-go/util/workqueue"

	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

var poolkey reflect.Type

const tick = 30 * time.Second
const tickCmd = "TICK"

func init() {
	poolkey, _ = utils.TypeKey((*Pool)(nil))
}

type reconcilers []reconcile.Interface

func (this reconcilers) add(reconciler reconcile.Interface) reconcilers {
	for _, r := range this {
		if r == reconciler {
			return this
		}
	}
	return append(this, reconciler)
}

type reconcilerMapping struct {
	values   map[interface{}]reconcilers
	matchers map[utils.Matcher]reconcilers
}

func newReconcilerMapping() *reconcilerMapping {
	return &reconcilerMapping{
		values:   map[interface{}]reconcilers{},
		matchers: map[utils.Matcher]reconcilers{},
	}
}

func (this *reconcilerMapping) getReconcilers(key interface{}) reconcilers {
	i := this.values[key]
	if i == nil {
		cmd, ok := key.(string)
		if ok {
			for m, i := range this.matchers {
				if m.Match(cmd) {
					return i
				}
			}
		}
		return nil
	}
	return i
}

func (this *reconcilerMapping) addReconciler(key ReconcilationElementSpec, reconciler reconcile.Interface) {
	switch k := key.(type) {
	case utils.StringMatcher:
		this.values[string(k)] = this.values[string(k)].add(reconciler)
	case utils.Matcher:
		this.matchers[k] = this.matchers[k].add(reconciler)
	default:
		this.values[k] = this.values[k].add(reconciler)
	}
}

type pool struct {
	logger.LogContext
	*controller
	name        string
	size        int
	ctx         context.Context
	period      time.Duration
	key         string
	workqueue   workqueue.RateLimitingInterface
	reconcilers *reconcilerMapping
}

func NewPool(controller *controller, name string, size int, period time.Duration) *pool {

	pool := &pool{
		name:        name,
		controller:  controller,
		size:        size,
		period:      period,
		key:         fmt.Sprintf("controller:%s/pool:%s", controller.GetName(), name),
		workqueue:   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), name),
		reconcilers: newReconcilerMapping(),
	}
	pool.ctx, pool.LogContext = logger.WithLogger(
		ctxutil.WaitGroupContext(
			context.WithValue(controller.GetContext(), poolkey, pool),
			fmt.Sprintf("pool %s of controller %s", name, controller.GetName())),
		"pool", name)
	if pool.period != 0 {
		pool.Infof("pool size %d, resync period %s", pool.size, pool.period.String())
	} else {
		pool.Infof("pool size %d", pool.size)
	}
	return pool
}

func (p *pool) whenReady() {
	p.controller.whenReady()
}

func (p *pool) addReconciler(key ReconcilationElementSpec, reconciler reconcile.Interface) {
	p.Infof("adding reconciler %T for key %q", reconciler, key)
	p.reconcilers.addReconciler(key, reconciler)
}

func (p *pool) getReconcilers(key interface{}) []reconcile.Interface {
	p.whenReady()
	return p.reconcilers.getReconcilers(key)
}

func (p *pool) GetName() string {
	return p.name
}

func (p *pool) GetWorkqueue() workqueue.RateLimitingInterface {
	return p.workqueue
}

func (p *pool) Key() string {
	return p.key
}

func (p *pool) Period() time.Duration {
	return p.period
}

func (p *pool) StartTicker() {
	// noop as periodic tick is always activated
}

func (p *pool) Run() {
	p.Infof("Starting worker pool with %d workers", p.size)
	period := p.period
	if period == 0 {
		p.Infof("no reconcile period active -> start ticker")
		period = tick
	}
	// always run periodic tickCmd to deal with empty workqueue
	p.workqueue.AddAfter(tickCmd, period)

	healthz.Start(p.Key(), period)
	for i := 0; i < p.size; i++ {
		p.startWorker(i, p.ctx.Done())
	}

	<-p.ctx.Done()
	p.workqueue.ShutDown()
	p.Infof("waiting for workers to shutdown")
	ctxutil.WaitGroupWait(p.ctx, 120*time.Second)
	healthz.End(p.Key())
}

func (p *pool) startWorker(number int, stopCh <-chan struct{}) {
	ctxutil.WaitGroupRunUntilCancelled(p.ctx, func() { newWorker(p, number).Run() })
}
func (p *pool) EnqueueCommand(cmd string) {
	p.enqueueCommand(cmd, p.workqueue.Add)
}
func (p *pool) EnqueueCommandRateLimited(name string) {
	p.enqueueCommand(name, p.workqueue.AddRateLimited)
}
func (p *pool) EnqueueCommandAfter(name string, duration time.Duration) {
	p.enqueueCommand(name, func(key interface{}) { p.workqueue.AddAfter(key, duration) })
}
func (p *pool) enqueueCommand(cmd string, add func(interface{})) {
	add(EncodeCommandKey(cmd))
}

func (p *pool) EnqueueKey(key resources.ClusterObjectKey) {
	p.enqueueKey(key, p.workqueue.Add)
}
func (p *pool) EnqueueKeyRateLimited(key resources.ClusterObjectKey) {
	p.enqueueKey(key, p.workqueue.AddRateLimited)
}
func (p *pool) EnqueueKeyAfter(key resources.ClusterObjectKey, duration time.Duration) {
	p.enqueueKey(key, func(key interface{}) { p.workqueue.AddAfter(key, duration) })
}
func (p *pool) enqueueKey(key resources.ClusterObjectKey, add func(interface{})) {
	cluster := p.GetClusterById(key.Cluster()).GetName()
	okey := EncodeObjectKey(cluster, key.ObjectKey())
	add(okey)
}

func (p *pool) EnqueueObject(obj resources.Object) {
	p.enqueueObject(obj, p.workqueue.Add)
}
func (p *pool) EnqueueObjectRateLimited(obj resources.Object) {
	p.enqueueObject(obj, p.workqueue.AddRateLimited)
}
func (p *pool) EnqueueObjectAfter(obj resources.Object, duration time.Duration) {
	p.enqueueObject(obj, func(key interface{}) { p.workqueue.AddAfter(key, duration) })
}

func (p *pool) enqueueObject(obj resources.Object, add func(interface{})) {
	if obj == nil {
		panic("cannot enqueue nil objects")
	}
	if obj.GetCluster() == nil {
		panic("cannot enqueue objects without a cluster")
	}

	key := EncodeObjectKeyForObject(obj)
	add(key)
}

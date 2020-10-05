/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/server/healthz"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/workqueue"
)

const DeletionActivity = controllermanager.DeletionActivity

// worker describe a single threaded worker entity synchronously working
// on requests provided by the controller workqueue
// It is basically a single go routine with a state for subsequent methods
// called from w go routine
type worker struct {
	logger.LogContext

	ctx        context.Context
	logContext logger.LogContext
	pool       *pool
	workqueue  workqueue.RateLimitingInterface
}

func newWorker(p *pool, number int) *worker {
	lgr := p.NewContext("worker", strconv.Itoa(number))

	return &worker{
		LogContext: lgr,

		ctx:        p.ctx,
		logContext: lgr,
		pool:       p,
		workqueue:  p.workqueue,
	}
}

func (w *worker) Run() {
	w.Infof("starting worker")
	for w.processNextWorkItem() {
	}
	w.Infof("exit worker")
}

func (w *worker) internalErr(obj interface{}, err error) bool {
	w.Error(err)
	w.workqueue.Forget(obj)
	return true
}

func (w *worker) loggerForKey(key string) func() {
	w.LogContext = w.logContext.NewContext("resources", key)
	return func() { w.LogContext = w.logContext }
}

func (w *worker) processNextWorkItem() bool {
	obj, shutdown := w.workqueue.Get()
	if shutdown {
		return false
	}
	w.Debugf("GOT: %s", obj)
	defer w.workqueue.Done(obj)
	defer w.Debugf("DONE %s", obj)
	healthz.Tick(w.pool.Key())

	key, ok := obj.(string)
	if !ok {
		return w.internalErr(obj, fmt.Errorf("expected string in workqueue but got %#v", obj))
	}

	defer w.loggerForKey(key)()

	cmd, rkey, r, err := w.pool.controller.DecodeKey(key)

	if err != nil {
		// The resources may no longer exist, in which case we stop processing.
		if !errors.IsNotFound(err) {
			w.Errorf("error syncing '%s': %s", key, err)
			w.workqueue.AddRateLimited(key)
			return true
		}
	}

	ok = true
	err = nil
	var reschedule time.Duration = -1
	if cmd != "" {
		reconcilers := w.pool.getReconcilers(cmd)
		if reconcilers != nil && len(reconcilers) > 0 {
			for _, reconciler := range reconcilers {
				status := reconciler.Command(w, cmd)
				if !status.Completed {
					ok = false
				}
				if status.Error != nil {
					err = status.Error
					w.Errorf("command %q failed: %s", cmd, err)
				}
				updateSchedule(&reschedule, status.Interval)
			}
		} else {
			if cmd == tickCmd {
				healthz.Tick(w.pool.Key())
				w.workqueue.AddAfter(tickCmd, tick)
			} else {
				w.Errorf("no reconciler found for command %q:", key)
			}
			return true
		}
	}
	deleted := false
	if rkey != nil {
		if r != nil {
			r = r.DeepCopy()
		}
		reconcilers := w.pool.getReconcilers(rkey.GroupKind())

		var f func(reconcile.Interface) reconcile.Status
		switch {
		case r == nil:
			deleted = true
			if w.pool.Owning().GroupKind() == (*rkey).GroupKind() {
				ctxutil.Tick(w.ctx, DeletionActivity)
			}
			f = func(reconciler reconcile.Interface) reconcile.Status { return reconciler.Deleted(w, *rkey) }
		case r.IsDeleting():
			deleted = true
			if w.pool.Owning().GroupKind() == r.GroupKind() {
				ctxutil.Tick(w.ctx, DeletionActivity)
			}
			f = func(reconciler reconcile.Interface) reconcile.Status { return reconciler.Delete(w, r) }
		default:
			f = func(reconciler reconcile.Interface) reconcile.Status { return reconciler.Reconcile(w, r) }
		}

		for _, reconciler := range reconcilers {
			status := f(reconciler)
			w.pool.controller.requestHandled(w, reconciler, *rkey)
			if !status.Completed {
				ok = false
			}
			if status.Error != nil {
				err = status.Error
				if ok && r != nil {
					r.Eventf(corev1.EventTypeWarning, "sync", "%s", err.Error())
				}
			}
			if status.Interval >= 0 {
				w.Debugf("requested reschedule %d seconds", status.Interval/time.Second)
			}
			updateSchedule(&reschedule, status.Interval)
		}

	}
	if err != nil {
		if ok && reschedule < 0 {
			w.Warnf("add rate limited because of problem: %s", err)
			// valid resources, but resources not ready yet (required state for reconciliation/deletion not yet) reached, re-add to the queue rate-limited
			w.workqueue.AddRateLimited(obj)
		} else {
			// invalid resources (not suitable for controller)
			if reschedule > 0 {
				w.Infof("request reschedule %q after %d seconds", obj, reschedule/time.Second)
				w.workqueue.AddAfter(obj, reschedule)
			} else {
				w.Infof("wait for new change '%s': %s", key, err)
			}
		}
	} else {
		if ok {
			// valid resources, everything ok, just continue normally
			w.workqueue.Forget(obj)
			if reschedule < 0 || (w.pool.Period() > 0 && w.pool.Period() < reschedule) {
				if !deleted {
					reschedule = w.pool.Period()
				}
			}

			if reschedule > 0 {
				if w.pool.Period() != reschedule {
					w.Infof("reschedule %q after %d seconds", obj, reschedule/time.Second)
				} else {
					w.Debugf("reschedule %q after %d seconds", obj, reschedule/time.Second)
				}
				w.workqueue.AddAfter(obj, reschedule)
			} else {
				if w.pool.Period() > 0 {
					w.Infof("stop reconciling %q", obj)
				} else {
					w.Debugf("stop reconciling %q", obj)
				}
			}
		} else {
			// valid resources, but reconciliation failed temporarily, just re-add to the queue
			w.Infof("redo reconcile %q", obj)
			w.workqueue.Add(obj)
		}
	}
	return true
}

func updateSchedule(reschedule *time.Duration, interval time.Duration) {
	if interval >= 0 && (*reschedule <= 0 || interval < *reschedule) {
		*reschedule = interval
	}
}

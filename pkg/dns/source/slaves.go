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
 *
 */

package source

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile/reconcilers"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/external-dns-management/pkg/dns"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns/utils"

	"k8s.io/apimachinery/pkg/api/errors"
)

func SlaveReconcilerType(c controller.Interface) (reconcile.Interface, error) {
	ownerState, err := getOrCreateSharedOwnerState(c, false)
	if err != nil {
		return nil, err
	}
	classes := controller.NewClassesByOption(c, OPT_CLASS, dns.CLASS_ANNOTATION, dns.DEFAULT_CLASS)
	reconciler := &slaveReconciler{
		controller:    c,
		slaves:        c.(*reconcilers.SlaveReconciler),
		targetClasses: controller.NewTargetClassesByOption(c, OPT_TARGET_CLASS, dns.CLASS_ANNOTATION, classes),
		events:        NewEvents(),
		state: c.GetOrCreateSharedValue(KEY_STATE,
			func() interface{} {
				return NewState(ownerState)
			}).(*state),
	}
	return reconciler, nil
}

type slaveReconciler struct {
	reconcile.DefaultReconciler
	controller    controller.Interface
	slaves        *reconcilers.SlaveReconciler
	targetClasses *controller.Classes
	events        *Events
	state         *state
}

func (this *slaveReconciler) Setup() error {
	return this.state.ownerState.Setup(this.controller)
}

func (this *slaveReconciler) Start() {
	this.controller.Infof("determining dangling dns entries...")
	cluster := this.controller.GetMainCluster()
	main := cluster.GetId()
	for k := range this.slaves.GetMasters(false) {
		if k.Cluster() == main {
			if _, err := cluster.GetCachedObject(k); errors.IsNotFound(err) {
				this.controller.Infof("trigger vanished origin %s", k.ObjectKey())
				this.controller.EnqueueKey(k)
			} else {
				this.controller.Debugf("found origin %s", k.ObjectKey())
			}
		}
	}
}

func (this *slaveReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	if !this.targetClasses.IsResponsibleFor(logger, obj) {
		return reconcile.Succeeded(logger)
	}

	stat := this.DefaultReconciler.Reconcile(logger, obj)

	logger.Infof("reconcile slave")
	entry := utils.DNSEntry(obj.DeepCopy())
	if entry != nil {
		for k := range this.slaves.Slaves().GetOwnersFor(obj.ClusterKey()) {
			logger.Infof("found owner %s", k)
			o, err := this.controller.GetObject(k)
			if err == nil {
				fb := this.state.CreateFeedbackForObject(o)
				if fb == nil {
					continue
				}
				s := entry.Status()
				n := entry.GetDNSName()

				stateCopy := DNSState{DNSEntryStatus: *s, CreationTimestamp: entry.GetCreationTimestamp()}
				if stateCopy.Provider != nil {
					str := "remote: " + *stateCopy.Provider
					stateCopy.Provider = &str
				} else {
					str := "remote"
					stateCopy.Provider = &str
				}

				logger.Infof("update event")
				switch s.State {
				case api.STATE_ERROR:
					msg := fmt.Errorf("errorneous dns entry")
					if s.Message != nil {
						msg = fmt.Errorf("%s: %s", msg, *s.Message)
					}
					fb.Failed(logger, n, msg, &stateCopy)
				case api.STATE_INVALID:
					msg := fmt.Errorf("dns entry invalid")
					if s.Message != nil {
						msg = fmt.Errorf("%s: %s", msg, *s.Message)
					}
					fb.Invalid(logger, n, msg, &stateCopy)
				case api.STATE_PENDING:
					msg := fmt.Sprintf("dns entry pending")
					if s.Message != nil {
						msg = fmt.Sprintf("%s: %s", msg, *s.Message)
					}
					fb.Pending(logger, n, msg, &stateCopy)
				case api.STATE_READY:
					if stateCopy.Message == nil {
						str := "dns entry ready"
						stateCopy.Message = &str
					}
					fb.Ready(logger, n, *stateCopy.Message, &stateCopy)
				}
			} else {
				logger.Debugf("owner %s not found: %s", k, err)
			}
		}
	}
	return stat
}

func (this *slaveReconciler) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	logger.Infof("delete slave %s", obj.ClusterKey())
	entry := utils.DNSEntry(obj)
	if entry != nil {
		for k := range this.slaves.Slaves().GetOwnersFor(obj.ClusterKey()) {
			logger.Infof("found owner %s", k)
			o, err := this.controller.GetObject(k)
			if err == nil {
				fb := this.state.CreateFeedbackForObject(o)
				if fb == nil {
					continue
				}
				n := entry.GetDNSName()
				fb.Deleted(logger, n, "")
			}
			this.events.Deleted(logger, k)
		}
	}
	return this.DefaultReconciler.Delete(logger, obj)
}

func (this *slaveReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	logger.Infof("deleted slave %s", key)
	for k := range this.slaves.Slaves().GetOwnersFor(key) {
		logger.Infof("found owner %s", k)
		_, err := this.controller.GetObject(k)
		if err == nil {
			fb := this.state.GetFeedback(k)
			if fb != nil {
				fb.Deleted(logger, "", "")
			}
		}
		this.state.DeleteFeedback(k)
	}
	return this.DefaultReconciler.Deleted(logger, key)
}

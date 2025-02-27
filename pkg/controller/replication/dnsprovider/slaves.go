// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dnsprovider

import (
	"fmt"
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile/reconcilers"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	utils2 "github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns/source"
	"github.com/gardener/external-dns-management/pkg/dns/utils"
)

const RemotePrefix = "remote: "

func SlaveReconcilerType(c controller.Interface) (reconcile.Interface, error) {
	gkSecret := resources.NewGroupKind("core", "Secret")
	resSecrets, err := c.GetCluster(source.TARGET_CLUSTER).Resources().GetByGK(gkSecret)
	if err != nil {
		return nil, err
	}
	reconciler := &slaveReconciler{
		controller: c,
		resSecrets: resSecrets,
		slaves:     c.(*reconcilers.SlaveReconciler),
	}
	return reconciler, nil
}

type slaveReconciler struct {
	reconcile.DefaultReconciler
	controller controller.Interface
	resSecrets resources.Interface
	slaves     *reconcilers.SlaveReconciler
}

func (this *slaveReconciler) Start() error {
	this.controller.Infof("determining dangling dns providers...")
	cluster := this.controller.GetMainCluster()
	main := cluster.GetId()
	for k := range this.slaves.GetMasters(false) {
		if k.Cluster() == main {
			if _, err := cluster.GetCachedObject(k); errors.IsNotFound(err) {
				this.controller.Infof("trigger vanished origin %s", k.ObjectKey())
				if err := this.controller.EnqueueKey(k); err != nil {
					return err
				}
			} else {
				this.controller.Debugf("found origin %s", k.ObjectKey())
			}
		}
	}
	return nil
}

func (this *slaveReconciler) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	stat := this.DefaultReconciler.Reconcile(logger, obj)

	logger.Infof("reconcile slave")
	provider := utils.DNSProvider(obj)
	if provider != nil && secretSetAndProcessed(provider.DNSProvider()) {
		status := provider.Status()
		for k := range this.slaves.Slaves().GetOwnersFor(obj.ClusterKey()) {
			logger.Infof("found owner %s", k)
			o, err := this.controller.GetObject(k)
			if err == nil {
				ownerStatus := utils.DNSProvider(o).Status()
				mod := &utils2.ModificationState{}
				mod.AssureStringValue(&ownerStatus.State, status.State)
				assureDNSSelectionStatus(mod, &ownerStatus.Domains, status.Domains)
				assureDNSSelectionStatus(mod, &ownerStatus.Zones, status.Zones)
				assureRateLimit(mod, &ownerStatus.RateLimit, status.RateLimit)
				mod.AssureInt64PtrPtr(&ownerStatus.DefaultTTL, status.DefaultTTL)
				var msg *string
				if status.Message != nil && *status.Message != "" {
					s := RemotePrefix + *status.Message
					msg = &s
				}
				mod.AssureStringPtrPtr(&ownerStatus.Message, msg)
				assureTimeValuePtrPtr(mod, &ownerStatus.LastUptimeTime, status.LastUptimeTime)
				if mod.IsModified() {
					err = o.UpdateStatus()
					if err != nil {
						return reconcile.DelayOnError(logger, fmt.Errorf("cannot update status of %s: %w", o.ObjectName(), err))
					}
				}
			} else {
				logger.Debugf("owner %s not found: %s", k, err)
			}
		}
	}
	return stat
}

func secretSetAndProcessed(provider *api.DNSProvider) bool {
	if provider.Spec.SecretRef == nil || provider.Status.Message == nil {
		return false
	}
	return *provider.Status.Message != "no secret specified"
}

func assureDNSSelectionStatus(mod *utils2.ModificationState, t *api.DNSSelectionStatus, s api.DNSSelectionStatus) {
	if !reflect.DeepEqual(*t, s) {
		*t = s
		mod.Modify(true)
	}
}

func assureRateLimit(mod *utils2.ModificationState, t **api.RateLimit, s *api.RateLimit) {
	if s == nil && *t != nil {
		*t = nil
		mod.Modify(true)
	} else if s != nil {
		if *t == nil || !reflect.DeepEqual(**t, *s) {
			*t = s
			mod.Modify(true)
		}
	}
}

func assureTimeValuePtrPtr(mod *utils2.ModificationState, t **metav1.Time, s *metav1.Time) {
	if s == nil && *t != nil {
		*t = nil
		mod.Modify(true)
	} else if s != nil {
		if *t == nil || s.Time != (*t).Time {
			*t = s
			mod.Modify(true)
		}
	}
}

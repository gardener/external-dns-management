/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"context"
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/lease"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"

	"k8s.io/client-go/tools/leaderelection"
)

type leasestartupgroup struct {
	startupgroup
}

func (this *leasestartupgroup) Startup() error {
	if len(this.controllers) == 0 {
		return nil
	}

	msg := this.cluster.GetName()
	sep := " ("
	for _, c := range this.controllers {
		msg = fmt.Sprintf("%s%s%s", msg, sep, c.GetName())
		sep = ", "
	}
	msg += ")"

	leasecfg := &this.extension.config.Lease
	this.extension.Infof("leader election required for %s", msg)
	runit := func() {
		this.extension.Infof("Acquired leadership, starting controllers for %s.", msg)
		for _, c := range this.controllers {
			this.extension.setupController(c)
		}
		for _, c := range this.controllers {
			this.extension.startController(c)
		}
	}

	if leasecfg.OmitLease {
		this.extension.Infof("omitting lease %q for cluster %s in namespace %q",
			this.extension.Name(), msg, this.extension.Namespace())
		ctxutil.WaitGroupRun(this.extension.GetContext(), runit)
	} else {
		this.extension.Infof("requesting lease %q for cluster %s in namespace %q",
			leasecfg.LeaseName, msg, this.extension.Namespace())
		leaderElectionConfig, err := lease.MakeLeaderElectionConfig(this.cluster,
			this.extension.Namespace(), &this.extension.config.Lease)
		if err != nil {
			return err
		}

		leaderElectionConfig.Callbacks = leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				go func() {
					<-ctx.Done()
					this.extension.Infof("lease group %s stopped -> shutdown controller manager", this.cluster.GetName())
					ctxutil.Cancel(this.extension.ControllerManager().GetContext())
				}()
				runit()
			},
			OnStoppedLeading: func() {
				this.extension.Infof("Lost leadership, cleaning up %s.", msg)
			},
		}
		leaderElector, err := leaderelection.NewLeaderElector(*leaderElectionConfig)
		if err != nil {
			return fmt.Errorf("couldn't create leader elector: %v", err)
		}
		ctxutil.WaitGroupRun(this.extension.GetContext(), func() { leaderElector.Run(this.extension.GetContext()) })
	}

	return nil
}

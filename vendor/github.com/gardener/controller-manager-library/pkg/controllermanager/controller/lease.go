/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"context"
	"fmt"
	"os"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/config"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"

	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
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

	this.extension.Infof("leader election required for %s", msg)
	runit := func() {
		this.extension.Infof("Acquired leadership, starting controllers for %s.", msg)
		for _, c := range this.controllers {
			this.extension.startController(c)
		}
	}

	if this.extension.config.OmitLease {
		this.extension.Infof("omitting lease %q for cluster %s in namespace %q",
			this.extension.Name(), msg, this.extension.Namespace())
		ctxutil.WaitGroupRun(this.extension.GetContext(), runit)
	} else {
		this.extension.Infof("requesting lease %q for cluster %s in namespace %q",
			this.extension.config.LeaseName, msg, this.extension.Namespace())
		leaderElectionConfig, err := makeLeaderElectionConfig(this.cluster,
			this.extension.Namespace(), this.extension.config)
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

func makeLeaderElectionConfig(cluster cluster.Interface, namespace string, ctrlconfig *config.Config) (*leaderelection.LeaderElectionConfig, error) {
	hostname, err := os.Hostname()
	hostname = fmt.Sprintf("%s/%d", hostname, os.Getpid())
	if err != nil {
		return nil, fmt.Errorf("unable to get hostname: %v", err)
	}

	cfg := cluster.Config()
	client, err := k8s.NewForConfig(&cfg)
	if err != nil {
		return nil, err
	}
	lock, err := resourcelock.New(
		"configmaps",
		namespace,
		ctrlconfig.LeaseName,
		client.CoreV1(),
		client.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity:      hostname,
			EventRecorder: cluster.Resources(),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("couldn't create resources lock: %v", err)
	}

	return &leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: ctrlconfig.LeaseDuration,
		RenewDeadline: ctrlconfig.LeaseRenewDeadline,
		RetryPeriod:   ctrlconfig.LeaseRetryPeriod,
	}, nil
}

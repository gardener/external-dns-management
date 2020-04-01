/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved.
 * This file is licensed under the Apache Software License, v. 2 except as noted
 * otherwise in the LICENSE file
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

package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
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
			this.extension.Namespace(), this.extension.config.LeaseName)
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

func makeLeaderElectionConfig(cluster cluster.Interface, namespace, name string) (*leaderelection.LeaderElectionConfig, error) {
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
		name,
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
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
	}, nil
}

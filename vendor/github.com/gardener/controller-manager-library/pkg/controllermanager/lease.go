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

package controllermanager

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"

	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

type leasestartupgroup struct {
	startupgroup
}

func (g *leasestartupgroup) Startup() error {
	for _, c := range g.controllers {
		err := g.manager.checkController(c)
		if err != nil {
			return err
		}
	}

	if len(g.controllers) == 0 {
		return nil
	}

	msg := g.cluster.GetName()
	sep := " ("
	for _, c := range g.controllers {
		msg = fmt.Sprintf("%s%s%s", msg, sep, c.GetName())
		sep = ", "
	}
	msg += ")"

	runit := func() {
		g.manager.Infof("Acquired leadership, starting controllers for %s.", msg)
		for _, c := range g.controllers {
			g.manager.startController(c)
		}
	}

	if g.manager.GetConfig().OmitLease {
		g.manager.Infof("omitting lease %q for cluster %s in namespace %q",
			g.manager.GetName(), msg, g.manager.GetConfig().Namespace)
		ctxutil.SyncPointRun(g.manager.ctx, runit)
	} else {
		g.manager.Infof("requesting lease %q for cluster %s in namespace %q",
			g.manager.GetName(), msg, g.manager.GetConfig().Namespace)
		leaderElectionConfig, err := makeLeaderElectionConfig(g.cluster,
			g.manager.GetConfig().Namespace, g.manager.GetName())
		if err != nil {
			return err
		}

		leaderElectionConfig.Callbacks = leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) { runit() },
			OnStoppedLeading: func() {
				g.manager.Infof("Lost leadership, cleaning up %s.", msg)
			},
		}
		leaderElector, err := leaderelection.NewLeaderElector(*leaderElectionConfig)
		if err != nil {
			return fmt.Errorf("couldn't create leader elector: %v", err)
		}
		ctxutil.SyncPointRun(g.manager.ctx, func() { leaderElector.Run(g.manager.ctx) })
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
	client, err := corev1.NewForConfig(&cfg)
	if err != nil {
		return nil, err
	}
	lock, err := resourcelock.New(
		"configmaps",
		namespace,
		name,
		client,
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

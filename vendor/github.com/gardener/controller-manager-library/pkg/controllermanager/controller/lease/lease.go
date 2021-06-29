/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package lease

import (
	"fmt"
	"os"

	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
)

func MakeLeaderElectionConfig(cluster cluster.Interface, namespace string, config *Config) (*leaderelection.LeaderElectionConfig, error) {
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
		config.LeaseLeaderElectionResourceLock,
		namespace,
		config.LeaseName,
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
		LeaseDuration: config.LeaseDuration,
		RenewDeadline: config.LeaseRenewDeadline,
		RetryPeriod:   config.LeaseRetryPeriod,
	}, nil
}

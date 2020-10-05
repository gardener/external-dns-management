/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package access

import (
	"fmt"
	"sync"

	"github.com/gardener/controller-manager-library/pkg/logger"
)

var (
	lock    sync.RWMutex
	entries = []*entry{}
)

type entry struct {
	priority int
	clusters map[string][]AccessController
}

func (this *entry) forCluster(clusterId string) []AccessController {
	list := this.clusters[clusterId]
	global := this.clusters[""]
	if list != nil {
		if global != nil {
			return append(append([]AccessController{}, list...), global...)
		}
		return list
	}
	return global
}

func Register(ctr AccessController, clusterId string, priority int) {
	if priority > MIN_PRIO || priority < MAX_PRIO {
		panic(fmt.Errorf("invalid access controller priority %d for %q", priority, ctr.Name()))
	}
	lock.Lock()
	defer lock.Unlock()

	if clusterId == "" {
		logger.Infof("registering global access controller %q with priority %d", ctr.Name(), priority)
	} else {
		logger.Infof("registering access controller %q for cluster %q with priority %d", ctr.Name(), clusterId, priority)
	}

	var found *entry
	for i, e := range entries {
		if e.priority == priority {
			found = e
			break
		}
		if e.priority > priority {
			found = &entry{priority, map[string][]AccessController{}}
			entries = append(entries[:i], append([]*entry{found}, entries[i:]...)...)
			break
		}
	}
	if found == nil {
		found = &entry{priority, map[string][]AccessController{}}
		entries = append(entries, found)
	}
	found.clusters[clusterId] = append(found.clusters[clusterId], ctr)
}

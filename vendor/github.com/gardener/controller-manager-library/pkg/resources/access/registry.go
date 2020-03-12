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

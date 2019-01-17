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

package healthz

import (
	"fmt"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
)

func Tick(key string) {
	lock.Lock()
	defer lock.Unlock()

	setCheck(key)
}

func Start(key string, period time.Duration) {
	lock.Lock()
	defer lock.Unlock()

	checks[key] = &check{time.Now(), 3 * period}
}

func End(key string) {
	lock.Lock()
	defer lock.Unlock()

	removeCheck(key)
}

type check struct {
	last    time.Time
	timeout time.Duration
}

var (
	checks = map[string]*check{}
	lock   sync.Mutex
)

func setCheck(key string) {
	c := checks[key]
	if c == nil {
		panic(fmt.Sprint("check with key %q not configured", key))
	}
	c.last = time.Now()
}

func removeCheck(key string) {
	delete(checks, key)
}

func IsHealthy() bool {
	lock.Lock()
	defer lock.Unlock()

	now := time.Now()

	for key, c := range checks {
		limit := now.Add(-c.timeout)
		if c.last.Before(limit) {
			logger.Warnf("outdated health check '%s': %s", key, limit.Sub(c.last))
			return false
		}
		logger.Debugf("%s: %s", key, c.last)
	}
	return true
}

func HealthInfo() (bool, string) {
	lock.Lock()
	defer lock.Unlock()

	info := ""
	now := time.Now()
	for key, c := range checks {
		limit := now.Add(-c.timeout)
		info = fmt.Sprintf("%s%s: %s\n", info, key, c.last)
		if c.last.Before(limit) {
			logger.Warnf("outdated health check '%s': %s", key, limit.Sub(c.last))
			return false, info
		}
		logger.Debugf("%s: %s", key, c.last)
	}
	return true, info
}

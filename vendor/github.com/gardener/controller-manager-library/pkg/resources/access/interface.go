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
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/errors"
)

const (
	ACCESS_PROCEED = iota
	ACCESS_GRANTED
	ACCESS_DENIED
)

const (
	MAX_PRIO = 0
	MIN_PRIO = 32768
)

type AccessController interface {
	Name() string
	Allowed(src resources.ClusterObjectKey, verb string, tgt resources.ClusterObjectKey) (int, error)
}

func Allowed(src resources.ClusterObjectKey, verb string, tgt resources.ClusterObjectKey) (bool, string, error) {
	logger.Debugf("checking access %s %s %s", src, verb, tgt)
	lock.RLock()
	defer lock.RUnlock()
	for _, e := range entries {
		for _, c := range e.forCluster(tgt.Cluster()) {
			a, err := c.Allowed(src, verb, tgt)
			logger.Debugf("%s: %s: checking access: %d, %s", tgt.Cluster(), c.Name(), a, err)
			if err != nil {
				return false, "error in " + c.Name(), err
			}
			switch a {
			case ACCESS_PROCEED:
			case ACCESS_GRANTED:
				return true, "granted by " + c.Name(), nil
			case ACCESS_DENIED:
				return false, "denied by " + c.Name(), nil
			default:
				return false, "denied by " + c.Name(), errors.New(errors.ERR_INVALID_RESPONSE, "invalid response from %s: %d", c.Name(), a)
			}
		}
	}
	return true, "", nil
}

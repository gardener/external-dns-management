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
	"sync"

	"github.com/gardener/controller-manager-library/pkg/logger"
)

type SharedAttributes interface {
	GetSharedValue(key interface{}) interface{}
	GetOrCreateSharedValue(key interface{}, create func() interface{}) interface{}
}

type sharedAttributes struct {
	logger.LogContext
	lock   sync.RWMutex
	shared map[interface{}]interface{}
}

func (c *sharedAttributes) GetSharedValue(key interface{}) interface{} {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.shared == nil {
		return nil
	}
	return c.shared[key]
}

func (c *sharedAttributes) GetOrCreateSharedValue(key interface{}, create func() interface{}) interface{} {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.shared == nil {
		c.shared = map[interface{}]interface{}{}
	}
	v, ok := c.shared[key]
	if !ok {
		c.Infof("creating shared value for key %v", key)
		v = create()
		c.shared[key] = v
	}
	return v
}

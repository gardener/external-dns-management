/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package extension

import (
	"sync"

	"github.com/gardener/controller-manager-library/pkg/logger"
)

func NewSharedAttributes(logger logger.LogContext) *SharedAttributesImpl {
	return &SharedAttributesImpl{LogContext: logger}
}

type SharedAttributesImpl struct {
	logger.LogContext
	lock   sync.RWMutex
	shared map[interface{}]interface{}
}

func (c *SharedAttributesImpl) GetSharedValue(key interface{}) interface{} {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.shared == nil {
		return nil
	}
	return c.shared[key]
}

func (c *SharedAttributesImpl) GetOrCreateSharedValue(key interface{}, create func() interface{}) interface{} {
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

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

package provider

import (
	"fmt"
	"strings"
)

func (c *DNSHandlerConfig) GetRequiredProperty(key string, altKeys ...string) (string, error) {
	return c.getProperty(key, true, altKeys...)
}

func (c *DNSHandlerConfig) GetProperty(key string, altKeys ...string) string {
	value, _ := c.getProperty(key, false, altKeys...)
	return value
}

func (c *DNSHandlerConfig) getProperty(key string, required bool, altKeys ...string) (string, error) {
	usedKey := key
	value, ok := c.Properties[key]
	if !ok && len(altKeys) > 0 {
		for _, altKey := range altKeys {
			value, ok = c.Properties[altKey]
			if ok {
				usedKey = altKey
				break
			}
		}
	}
	if !ok {
		if !required {
			return "", nil
		}
		keys := append([]string{key}, altKeys...)
		err := fmt.Errorf("'%s' required in secret", strings.Join(keys, "' or '"))
		return "", err
	}

	tvalue := strings.TrimSpace(value)
	if value != tvalue {
		c.Logger.Warnf("value for '%s' in secret contains leading or trailing spaces which have been removed", usedKey)
	}
	if tvalue == "" {
		if !required {
			return "", nil
		}
		err := fmt.Errorf("value for '%s' in secret is empty", usedKey)
		return "", err
	}
	return tvalue, nil
}

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
	"strconv"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

func (c *DNSHandlerConfig) GetRequiredProperty(key string, altKeys ...string) (string, error) {
	return c.getProperty(key, true, altKeys...)
}

func (c *DNSHandlerConfig) GetProperty(key string, altKeys ...string) string {
	value, _ := c.getProperty(key, false, altKeys...)
	return value
}

func (c *DNSHandlerConfig) GetRequiredIntProperty(key string, altKeys ...string) (int, error) {
	value, err := c.getProperty(key, true, altKeys...)
	if err!=nil {
		return 0, err
	}
	return strconv.Atoi(value)
}

func (c *DNSHandlerConfig) GetDefaultedProperty(key string, def string, altKeys ...string) string {
	value, _ := c.getProperty(key, false, altKeys...)
	if value=="" {
		return def
	}
	return value
}

func (c *DNSHandlerConfig) GetDefaultedIntProperty(key string, def int, altKeys ...string) (int, error) {
	value, _ := c.getProperty(key, false, altKeys...)
	if value=="" {
		return def, nil
	}
	return strconv.Atoi(value)
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


func (c *DNSHandlerConfig) FillRequiredProperty(target **string, prop string, alt ...string) error {
	if utils.IsEmptyString(*target) {
		value, err := c.GetRequiredProperty(prop, alt...)
		if err != nil {
			return err
		}
		*target = &value
	} else {
		value := c.GetProperty(prop, alt...)
		if value != "" {
			return  fmt.Errorf("version defined in secret and provider config")
		}
	}
	return nil
}

func (c *DNSHandlerConfig)  FillRequiredIntProperty(target **int, prop string, alt ...string) error {
	if *target==nil || **target==0 {
		value, err := c.GetRequiredProperty(prop, alt...)
		if err != nil {
			return err
		}
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("property %s must be an int value: %s", prop, err)
		}
		*target = &i
	} else {
		value := c.GetProperty(prop, alt...)
		if value != "" {
			return  fmt.Errorf("property %s defined in secret and provider config", prop)
		}
	}
	return nil
}

func (c *DNSHandlerConfig)  FillDefaultedIntProperty(target **int, def int, prop string, alt ...string) error {
	if *target==nil || **target==0 {
		value := c.GetProperty(prop, alt...)
		if value=="" {
			*target=&def
		}
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("property %s must be an int value: %s", err)
		}
		*target = &i
	} else {
		value := c.GetProperty(prop, alt...)
		if value != "" {
			return  fmt.Errorf("%s defined in secret and provider config", prop)
		}
	}
	return nil
}
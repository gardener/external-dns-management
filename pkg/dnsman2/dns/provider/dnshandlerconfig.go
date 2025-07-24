// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// DNSHandlerConfig holds configuration for creating a DNSHandler.
type DNSHandlerConfig struct {
	Log          logr.Logger
	Properties   utils.Properties
	Config       *runtime.RawExtension
	GlobalConfig config.DNSManagerConfiguration
	Metrics      Metrics
	RateLimiter  flowcontrol.RateLimiter
}

// GetRequiredProperty returns the value for the given key or alternative keys, or an error if not found.
func (c *DNSHandlerConfig) GetRequiredProperty(key string, altKeys ...string) (string, error) {
	return c.getProperty(key, true, altKeys...)
}

// GetProperty returns the value for the given key or alternative keys, or an empty string if not found.
func (c *DNSHandlerConfig) GetProperty(key string, altKeys ...string) string {
	value, _ := c.getProperty(key, false, altKeys...)
	return value
}

// GetRequiredIntProperty returns the int value for the given key or alternative keys, or an error if not found or not an int.
func (c *DNSHandlerConfig) GetRequiredIntProperty(key string, altKeys ...string) (int, error) {
	value, err := c.getProperty(key, true, altKeys...)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(value)
}

// GetRequiredBoolProperty returns the bool value for the given key or alternative keys, or an error if not found or not a bool.
func (c *DNSHandlerConfig) GetRequiredBoolProperty(key string, altKeys ...string) (bool, error) {
	value, err := c.getProperty(key, true, altKeys...)
	if err != nil {
		return false, err
	}
	return strconv.ParseBool(value)
}

// GetDefaultedProperty returns the value for the given key or alternative keys, or the default if not found.
func (c *DNSHandlerConfig) GetDefaultedProperty(key string, def string, altKeys ...string) string {
	value, _ := c.getProperty(key, false, altKeys...)
	if value == "" {
		return def
	}
	return value
}

// GetDefaultedIntProperty returns the int value for the given key or alternative keys, or the default if not found.
func (c *DNSHandlerConfig) GetDefaultedIntProperty(key string, def int, altKeys ...string) (int, error) {
	value, _ := c.getProperty(key, false, altKeys...)
	if value == "" {
		return def, nil
	}
	return strconv.Atoi(value)
}

// GetDefaultedBoolProperty returns the bool value for the given key or alternative keys, or the default if not found.
func (c *DNSHandlerConfig) GetDefaultedBoolProperty(key string, def bool, altKeys ...string) (bool, error) {
	value, _ := c.getProperty(key, false, altKeys...)
	if value == "" {
		return def, nil
	}
	return strconv.ParseBool(value)
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
		c.Log.Info("warning: value in secret contains leading or trailing spaces which have been removed", "key", usedKey)
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

// FillRequiredProperty sets the target to the required property value if not already set, or returns an error.
func (c *DNSHandlerConfig) FillRequiredProperty(target **string, prop string, alt ...string) error {
	if ptr.Deref(*target, "") == "" {
		value, err := c.GetRequiredProperty(prop, alt...)
		if err != nil {
			return err
		}
		*target = &value
		return nil
	}
	return c.checkNotDefinedInConfig(prop, alt...)
}

// FillRequiredIntProperty sets the target to the required int property value if not already set, or returns an error.
func (c *DNSHandlerConfig) FillRequiredIntProperty(target **int, prop string, alt ...string) error {
	if *target == nil || **target == 0 {
		value, err := c.GetRequiredProperty(prop, alt...)
		if err != nil {
			return err
		}
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("property %s must be an int value: %s", prop, err)
		}
		*target = &i
		return nil
	}
	return c.checkNotDefinedInConfig(prop, alt...)
}

// FillDefaultedIntProperty sets the target to the int property value or default if not already set.
func (c *DNSHandlerConfig) FillDefaultedIntProperty(target **int, def int, prop string, alt ...string) error {
	if *target == nil || **target == 0 {
		value := c.GetProperty(prop, alt...)
		if value == "" {
			*target = &def
			return nil
		}
		i, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("property %s must be an int value: %s", prop, err)
		}
		*target = &i
		return nil
	}
	return c.checkNotDefinedInConfig(prop, alt...)
}

// FillDefaultedProperty sets the target to the property value or default if not already set.
func (c *DNSHandlerConfig) FillDefaultedProperty(target **string, def string, prop string, alt ...string) error {
	if *target == nil || **target == "" {
		value := c.GetProperty(prop, alt...)
		if value == "" {
			if def == "" {
				*target = nil
				return nil
			}
			*target = &def
			return nil
		}
		*target = &value
		return nil
	}
	return c.checkNotDefinedInConfig(prop, alt...)
}

// FillDefaultedBoolProperty sets the target to the bool property value or default if not already set.
func (c *DNSHandlerConfig) FillDefaultedBoolProperty(target **bool, def bool, prop string, alt ...string) error {
	if *target == nil || **target == def {
		value := c.GetProperty(prop, alt...)
		if value == "" {
			*target = &def
			return nil
		}
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		*target = &b
		return nil
	}
	return c.checkNotDefinedInConfig(prop, alt...)
}

func (c *DNSHandlerConfig) checkNotDefinedInConfig(prop string, alt ...string) error {
	value := c.GetProperty(prop, alt...)
	if value != "" {
		return fmt.Errorf("property %s defined in secret and provider config", prop)
	}
	return nil
}

// SetRateLimiter configures the rate limiter for the DNSHandlerConfig based on the provided options.
func (c *DNSHandlerConfig) SetRateLimiter(configuredRateLimiter, defaultRateLimiter *config.RateLimiterOptions, clock clock.Clock) error {
	c.RateLimiter = flowcontrol.NewFakeAlwaysRateLimiter()
	rateLimiterConfig := configuredRateLimiter
	if rateLimiterConfig == nil {
		rateLimiterConfig = defaultRateLimiter
	}
	if rateLimiterConfig != nil && rateLimiterConfig.Enabled {
		c.RateLimiter = flowcontrol.NewTokenBucketRateLimiterWithClock(float32(rateLimiterConfig.QPS), rateLimiterConfig.Burst, clock)
		c.Log.Info("rate limiter", "config", rateLimiterConfig)
	} else {
		c.Log.Info("no rate limiter configured or enabled, using always rate limiter")
	}
	return nil
}

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

// FactoryOptions is a set of generic options and
// type specific options related to
// factories that are provided for all provider types but in a
// provider specific manner.
// The provide specif options are defined by an option source
// optionally offered by the provider factory.
// It is handled by the compound factory, also, to provide
// dedicated option sets for hosted sub factories.
type FactoryOptions struct {
	// type specific options
	Options config.OptionSource
	// generic options for all factory types
	GenericFactoryOptions
}

var _ config.OptionSource = &FactoryOptions{}

func (this *FactoryOptions) AddOptionsToSet(set config.OptionSet) {
	// any generic option
	this.GenericFactoryOptions.AddOptionsToSet(set)

	// specific options for dedicated factory
	if this.Options != nil {
		this.Options.AddOptionsToSet(set)
	}
}

func (this *FactoryOptions) Evaluate() error {
	if this.Options != nil {
		if e, ok := this.Options.(config.OptionEvaluator); ok {
			return e.Evaluate()
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

type GenericFactoryOptions struct {
	RateLimiterOptions
	AdvancedOptions
}

var GenericFactoryOptionDefaults = GenericFactoryOptions{
	RateLimiterOptions: RateLimiterOptionDefaults,
	AdvancedOptions:    AdvancedOptionsDefaults,
}

func (this *GenericFactoryOptions) AddOptionsToSet(set config.OptionSet) {
	this.RateLimiterOptions.AddOptionsToSet(set)
	this.AdvancedOptions.AddOptionsToSet(set)
}

func (this GenericFactoryOptions) SetRateLimiterOptions(o RateLimiterOptions) GenericFactoryOptions {
	this.RateLimiterOptions = o
	return this
}

func (this GenericFactoryOptions) SetAdvancedOptions(o AdvancedOptions) GenericFactoryOptions {
	this.AdvancedOptions = o
	return this
}

////////////////////////////////////////////////////////////////////////////////

func (c *DNSHandlerConfig) GetRequiredProperty(key string, altKeys ...string) (string, error) {
	return c.getProperty(key, true, altKeys...)
}

func (c *DNSHandlerConfig) GetProperty(key string, altKeys ...string) string {
	value, _ := c.getProperty(key, false, altKeys...)
	return value
}

func (c *DNSHandlerConfig) GetRequiredIntProperty(key string, altKeys ...string) (int, error) {
	value, err := c.getProperty(key, true, altKeys...)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(value)
}

func (c *DNSHandlerConfig) GetRequiredBoolProperty(key string, altKeys ...string) (bool, error) {
	value, err := c.getProperty(key, true, altKeys...)
	if err != nil {
		return false, err
	}
	return strconv.ParseBool(value)
}

func (c *DNSHandlerConfig) GetDefaultedProperty(key string, def string, altKeys ...string) string {
	value, _ := c.getProperty(key, false, altKeys...)
	if value == "" {
		return def
	}
	return value
}

func (c *DNSHandlerConfig) GetDefaultedIntProperty(key string, def int, altKeys ...string) (int, error) {
	value, _ := c.getProperty(key, false, altKeys...)
	if value == "" {
		return def, nil
	}
	return strconv.Atoi(value)
}

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
		return nil
	}
	return c.checkNotDefinedInConfig(prop, alt...)
}

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

func (c *DNSHandlerConfig) Complete() error {
	rateLimiter := AlwaysRateLimiter()
	if c.Options != nil {
		rateLimiterConfig := c.Options.GetRateLimiterConfig()
		var err error
		rateLimiter, err = rateLimiterConfig.NewRateLimiter()
		if err != nil {
			return fmt.Errorf("invalid rate limiter: %w", err)
		}
		c.Logger.Infof("rate limiter: %v", rateLimiterConfig)
	}
	c.RateLimiter = rateLimiter
	return nil
}

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/utils"
	"k8s.io/apimachinery/pkg/util/sets"
)

// DNSHandlerAdapterChecks is a collection of checks for properties of a DNS handler adapter.
type DNSHandlerAdapterChecks struct {
	propertyChecks       []propertyCheck
	disjunctPropertySets [][]string
}

type propertyCheck struct {
	name       string
	aliases    []string
	required   bool
	allowEmpty bool
	hide       bool
	validators []PropertyValidator
}

// NewDNSHandlerAdapterChecks creates a new instance of DNSHandlerAdapterChecks.
func NewDNSHandlerAdapterChecks() *DNSHandlerAdapterChecks {
	return &DNSHandlerAdapterChecks{}
}

// DNSHandlerAdapterBuilder is a builder for creating property checks for DNS handler adapters.
type DNSHandlerAdapterBuilder struct {
	check propertyCheck
}

// Validators adds validators to the property check.
func (b *DNSHandlerAdapterBuilder) Validators(validators ...PropertyValidator) *DNSHandlerAdapterBuilder {
	b.check.validators = append(b.check.validators, validators...)
	return b
}

// AllowEmptyValue marks the property as allowed to be empty, meaning it will not trigger an error if the value is empty.
func (b *DNSHandlerAdapterBuilder) AllowEmptyValue() *DNSHandlerAdapterBuilder {
	b.check.allowEmpty = true
	return b
}

// HideValue marks the property as a secret or as too long, which means it should not be logged or displayed in any way.
func (b *DNSHandlerAdapterBuilder) HideValue() *DNSHandlerAdapterBuilder {
	b.check.hide = true
	return b
}

// RequiredProperty creates a new DNSHandlerAdapterBuilder for a required property with the given name and optional aliases.
func RequiredProperty(name string, aliases ...string) *DNSHandlerAdapterBuilder {
	return &DNSHandlerAdapterBuilder{
		check: propertyCheck{
			name:     name,
			aliases:  aliases,
			required: true,
		},
	}
}

// OptionalProperty creates a new DNSHandlerAdapterBuilder for an optional property with the given name and optional aliases.
func OptionalProperty(name string, aliases ...string) *DNSHandlerAdapterBuilder {
	return &DNSHandlerAdapterBuilder{
		check: propertyCheck{
			name:     name,
			aliases:  aliases,
			required: false,
		},
	}
}

// Add adds the property check to the DNSHandlerAdapterChecks instance.
// Typically, it is used in the form Add(RequiredProperty("name").Validators(...)).
func (c *DNSHandlerAdapterChecks) Add(b *DNSHandlerAdapterBuilder) {
	c.propertyChecks = append(c.propertyChecks, b.check)
}

// SetDisjunctPropertySets sets the disjunct property sets for the DNSHandlerAdapterChecks instance.
func (c *DNSHandlerAdapterChecks) SetDisjunctPropertySets(sets ...[]string) {
	c.disjunctPropertySets = sets
}

// HasPropertyNameOrAlias checks if the given properties contain a property with the specified name or any of its aliases.
func (c *DNSHandlerAdapterChecks) HasPropertyNameOrAlias(props utils.Properties, nameOrAlias string) bool {
	var check *propertyCheck
outer:
	for _, chk := range c.propertyChecks {
		if chk.name == nameOrAlias {
			check = &chk
			break outer
		}
		for _, alias := range chk.aliases {
			if alias == nameOrAlias {
				check = &chk
				break outer
			}
		}
	}
	if check == nil {
		return false
	}
	if props.Has(check.name) {
		return true
	}
	return slices.ContainsFunc(check.aliases, props.Has)
}

// ValidateProperties validates the properties against the defined checks.
func (c *DNSHandlerAdapterChecks) ValidateProperties(providerType string, properties utils.Properties) error {
	var (
		errs            []error
		allowedKeys     = sets.Set[string]{}
		duplicateKeys   = map[string]int{}
		foundProperties = sets.Set[string]{}
	)

	for idx, check := range c.propertyChecks {
		name := check.name
		value, found := properties[check.name]
		for _, alias := range check.aliases {
			if found {
				duplicateKeys[alias] = idx
			} else {
				value, found = properties[alias]
				if found {
					name = alias
				}
			}
		}

		allowedKeys.Insert(name)

		if !found {
			if check.required {
				errs = append(errs, fmt.Errorf("property %q is required but not provided", niceName(check.name, name)))
			}
			continue
		}
		foundProperties.Insert(check.name)

		if value == "" && !check.allowEmpty {
			if check.required {
				errs = append(errs, fmt.Errorf("property %q is required but empty", niceName(check.name, name)))
			} else {
				errs = append(errs, fmt.Errorf("property %q is empty (please set non-empty value or drop the property)", niceName(check.name, name)))
			}
			continue
		}

		for _, validator := range check.validators {
			if err := validator(value); err != nil {
				var printValue string
				if !check.hide {
					printValue = fmt.Sprintf(" with value %q", value)
				}
				msg := fmt.Sprintf("validation failed for property %s%s: %s", niceName(check.name, name), printValue, err)
				if check.hide {
					msg = strings.ReplaceAll(msg, value, "(hidden)")
				}
				return errors.New(msg)
			}
		}
	}

	for key, value := range properties {
		if !allowedKeys.Has(key) {
			if idx, found := duplicateKeys[key]; found {
				mismatching := false
				if v, ok := properties[c.propertyChecks[idx].name]; ok && v != value {
					mismatching = true
				}
				for _, alias := range c.propertyChecks[idx].aliases {
					if v, ok := properties[alias]; ok && v != value {
						mismatching = true
					}
				}
				if mismatching {
					errs = append(errs, fmt.Errorf("property %q is defined multiple times by an alias of %s", key, niceNameAndAliases(c.propertyChecks[idx])))
				}
			} else {
				errs = append(errs, fmt.Errorf("property %q is not allowed", key))
			}
		}
	}

	if err := c.validateDisjunctSets(foundProperties); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation failed for provider type %s: %s", providerType, errors.Join(errs...))
	}
	return nil
}

func (c *DNSHandlerAdapterChecks) validateDisjunctSets(foundProperties sets.Set[string]) error {
	if len(c.disjunctPropertySets) == 0 {
		return nil
	}

	validSetsFound := 0
	partialSetsFound := 0
	for _, propertySet := range c.disjunctPropertySets {
		count := 0
		for _, propName := range propertySet {
			if foundProperties.Has(propName) {
				count++
			}
		}
		if count == len(propertySet) {
			validSetsFound++
		} else if count > 0 {
			partialSetsFound++
		}
	}
	if validSetsFound != 1 || partialSetsFound > 0 {
		checks := map[string]propertyCheck{}
		for _, check := range c.propertyChecks {
			checks[check.name] = check
		}

		var setStrings []string
		for _, propertySet := range c.disjunctPropertySets {
			extendedPropertySet := make([]string, len(propertySet))
			for i, propName := range propertySet {
				if check, ok := checks[propName]; ok {
					extendedPropertySet[i] = niceNameAndAliases(check)
				} else {
					extendedPropertySet[i] = propName
				}
			}
			setStrings = append(setStrings, fmt.Sprintf("[%s]", strings.Join(extendedPropertySet, ", ")))
		}
		return fmt.Errorf("at least one of the disjunct property sets must be fully provided: %s",
			strings.Join(setStrings, " or "))
	}
	return nil
}

func niceName(preferredName, usedName string) string {
	if preferredName == usedName {
		return preferredName
	}
	return fmt.Sprintf("%s (alias for %s)", usedName, preferredName)
}

func niceNameAndAliases(pc propertyCheck) string {
	if len(pc.aliases) == 0 {
		return pc.name
	}
	return fmt.Sprintf("%s (aliases [%s])", pc.name, strings.Join(pc.aliases, ","))
}

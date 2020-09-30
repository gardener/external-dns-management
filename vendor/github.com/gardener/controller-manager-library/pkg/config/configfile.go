/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package config

import (
	"fmt"
	"io/ioutil"

	"github.com/ghodss/yaml"
	"github.com/spf13/pflag"
)

// MergeConfigFile reads a yaml or json config file and merges it into a given
// flagset
func MergeConfigFile(fileName string, flags *pflag.FlagSet, override bool) error {
	args, err := ReadConfigFile(fileName, flags)
	if err != nil {
		return fmt.Errorf("invalid config file %q; %s", fileName, err)
	}
	err = MergeFlags(flags, args, override)
	if err != nil {
		return fmt.Errorf("invalid config file %q; %s", fileName, err)
	}
	return nil
}

// MergeConfig merges a flagset with yaml or json config data
func MergeConfig(yaml []byte, flags *pflag.FlagSet, override bool) error {
	args, err := ParseConfig(yaml, flags)
	if err != nil {
		return fmt.Errorf("invalid config data: %s", err)
	}
	err = MergeFlags(flags, args, override)
	if err != nil {
		return fmt.Errorf("invalid config: %s", err)
	}
	return nil
}

// MergeFlags adds arguments to a flag set.
// Arguments that are already defined in the flag set are ignored
// if override is set to false
// If the flag is a slice override appends and does NOT replace the value.
// The pflag API does not allow to replace a value.
func MergeFlags(flags *pflag.FlagSet, args []string, override bool) error {
	valid := map[string]bool{}
	return flags.ParseAll(args, func(flag *pflag.Flag, value string) error {
		if override || !flag.Changed || valid[flag.Name] {
			valid[flag.Name] = true
			flags.Set(flag.Name, value)
		}
		return nil
	})
}

// ReadConfigFile reads a yaml or json file are parses its content to
// a list of equivalent command line arguments
func ReadConfigFile(name string, flags *pflag.FlagSet) ([]string, error) {
	bytes, err := ioutil.ReadFile(name)

	if err != nil {
		return nil, err
	}
	return ParseConfig(bytes, flags)
}

// ParseConfig parses  yaml or json data to a list of equivalent command line arguments
func ParseConfig(bytes []byte, flags *pflag.FlagSet) ([]string, error) {
	var data map[string]interface{}
	err := yaml.Unmarshal(bytes, &data)
	if err != nil {
		return nil, err
	}
	return MapToArguments("", flags, data)
}

func mapDataToArguments(name string, flags *pflag.FlagSet, data interface{}) ([]string, error) {
	arg := ""

	switch a := data.(type) {
	case string:
		arg = fmt.Sprintf("%s=%s", name, a)
	case bool:
		arg = fmt.Sprintf("%s=%t", name, a)
	case int64:
		arg = fmt.Sprintf("%s=%d", name, a)
	case float64:
		var i int64
		i = int64(a)
		if float64(i) == a {
			arg = fmt.Sprintf("%s=%d", name, i)
		} else {
			arg = fmt.Sprintf("%s=%f", name, a)
		}

	case []interface{}:
		var args []string
		for _, v := range a {
			sub, err := mapDataToArguments(name, flags, v)
			if err != nil {
				return nil, err
			}
			args = append(args, sub...)
		}
		return args, nil
	case map[string]interface{}:
		return MapToArguments(name, flags, a)
	default:
		return nil, fmt.Errorf("invalid type %T", data)
	}
	dash := "--"
	if flags != nil {
		if f := flags.Lookup(name); f != nil {
			if f.Shorthand == name {
				dash = "-"
			}
		} else {
			return nil, fmt.Errorf("invalid argument %q", name)
		}
	}
	return []string{dash + arg}, nil
}

func MapToArguments(prefix string, flags *pflag.FlagSet, data map[string]interface{}) ([]string, error) {
	var args []string

	for k, v := range data {
		name := k
		if name == "_" {
			if prefix != "" {
				return nil, fmt.Errorf("name '_' not possible at top level")
			}
			name = prefix
		} else {
			if prefix != "" {
				name = prefix + "." + name
			}
		}
		sub, err := mapDataToArguments(name, flags, v)
		if err != nil {
			return nil, err
		}
		args = append(args, sub...)
	}
	return args, nil
}

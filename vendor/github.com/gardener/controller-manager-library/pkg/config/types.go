/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package config

import (
	"time"

	"github.com/spf13/pflag"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

type OptionType interface {
	AddToFlags(flags *pflag.FlagSet, target interface{}, name, short string, def interface{}, desc string) interface{}
}

func optionTypeImpl(f func(flags *pflag.FlagSet, target interface{}, name, short string, def interface{}, desc string) interface{}) OptionType {
	return &optionType{f}
}

type optionType struct {
	f func(flags *pflag.FlagSet, target interface{}, name, short string, def interface{}, desc string) interface{}
}

func (t optionType) AddToFlags(flags *pflag.FlagSet, target interface{}, name, short string, def interface{}, desc string) interface{} {
	return t.f(flags, target, name, short, def, desc)
}

var (
	StringOption      = optionTypeImpl(tStringOption)
	StringArrayOption = optionTypeImpl(tStringArrayOption)
	IntOption         = optionTypeImpl(tIntOption)
	UintOption        = optionTypeImpl(tUintOption)
	BoolOption        = optionTypeImpl(tBoolOption)
	DurationOption    = optionTypeImpl(tDurationOption)
)

func tStringOption(flags *pflag.FlagSet, target interface{}, name, short string, def interface{}, desc string) interface{} {
	if def == nil {
		def = ""
	}
	if !utils.IsNil(target) {
		flags.StringVarP(target.(*string), name, short, def.(string), desc)
		return target
	}
	return flags.StringP(name, short, def.(string), desc)
}

func tStringArrayOption(flags *pflag.FlagSet, target interface{}, name, short string, def interface{}, desc string) interface{} {
	if def == nil {
		def = []string(nil)
	}
	if !utils.IsNil(target) {
		flags.StringArrayVarP(target.(*[]string), name, short, def.([]string), desc)
		return target
	}
	return flags.StringArrayP(name, short, def.([]string), desc)
}

func tIntOption(flags *pflag.FlagSet, target interface{}, name, short string, def interface{}, desc string) interface{} {
	if def == nil {
		def = int(0)
	}
	if !utils.IsNil(target) {
		flags.IntVarP(target.(*int), name, short, def.(int), desc)
		return target
	}
	return flags.IntP(name, short, def.(int), desc)
}

func tUintOption(flags *pflag.FlagSet, target interface{}, name, short string, def interface{}, desc string) interface{} {
	if def == nil {
		def = uint(0)
	}
	if !utils.IsNil(target) {
		flags.UintVarP(target.(*uint), name, short, def.(uint), desc)
		return target
	}
	return flags.UintP(name, short, def.(uint), desc)
}

func tBoolOption(flags *pflag.FlagSet, target interface{}, name, short string, def interface{}, desc string) interface{} {
	if def == nil {
		def = false
	}
	if !utils.IsNil(target) {
		flags.BoolVarP(target.(*bool), name, short, def.(bool), desc)
		return target
	}
	return flags.BoolP(name, short, def.(bool), desc)
}

func tDurationOption(flags *pflag.FlagSet, target interface{}, name, short string, def interface{}, desc string) interface{} {
	if def == nil {
		def = time.Duration(0)
	}
	if !utils.IsNil(target) {
		flags.DurationVarP(target.(*time.Duration), name, short, def.(time.Duration), desc)
		return target
	}
	return flags.DurationP(name, short, def.(time.Duration), desc)
}

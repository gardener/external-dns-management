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

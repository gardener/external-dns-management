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

// Package config allows to build up trees of option sources using the
// OptionSet interface. The tree is built up by calling the AddSource
// methods. This function just composes the tree, it does not propagate
// option declarations. Each node may therefore contain explicitly declared
// options.
// The option declarations are propgated up the tree if the it is
// explicitly completed or added to a flagset (AddToFlagSet) or another
// OptionSet (AddOptionsToSet). Once propagated the nodes cannot be modified
// anymore.
// This propagation may use the local option declarations to decide how
// those options should be visible in the next upper OptionSet.
// This package offers support for three kinds of propagations:
// - Flat:  the optiuons is just propagated as it is
// - Prefixed: the option is propagated with a prefix (it changes its name)
// - Shared: The option is propagated as it is but shared with other parallel
//          OptionSources
// - PrefixedShared: The option is propagated with a prefix. Additionally a shared
//          Option with the flat name is propagated.
//
// Once the the options are finally set by parsing a command line using a
// pflag.FlagSet the top level OptionSet must be evaluated calling the Evaluate
// method. This method may do semnatic checks but the main task is to
// propagate the settings down the tree. For Flat and Prefixed propagations
// there is always a dedicated single optio target directly filled by the FlagSet.
// But for shared options there might be multiple targets. This propagation
// is the task of the Evalue methods.
package config

import (
	"time"

	"github.com/spf13/pflag"
)

// OptionSource is the interface used to add arbitrary options to an OptionSet.
type OptionSource interface {
	// AddOptionsToSet must complete the actual source if it contains nested
	// option sources (implements OptionSourceSource) to figure out all relevant
	// options to add. This can be done explicitly or implicitly by forwording
	// the call to nested sources.
	AddOptionsToSet(set OptionSet)
}

// OptionCompleter must be impmented by OptionSource to execute code to
// complete the option set. It should lock the object for further changes
// of described options. Basically it must be called by the AddOptionsToSet
// prior to handling the actual option declarations if nested targets are
// involved to call AddOptionsToSet on nested targets to complete the actal set.
type OptionCompleter interface {
	Complete()
}

// OptionEvaluator must be implemented by OptionSource to execute code after
// option parsing is completed check and evaluate the actual settings
//
type OptionEvaluator interface {
	Evaluate() error
}

// OptionSourceVisitor is used to visit sources in an OptionGroup
type OptionSourceVisitor func(string, OptionSource) bool

// OptionSourceSource is a group of OptionSource
type OptionSourceSource interface {
	Name() string
	VisitSources(OptionSourceVisitor)
	GetSource(key string) OptionSource
}

// OptionSourceGroup is a modifyable group of OptionSource
type OptionSourceGroup interface {
	OptionSourceSource
	AddSource(key string, src OptionSource)
}

type OptionVisitor func(*ArbitraryOption) bool

// Options is an element offering named options
type Options interface {
	// GetOption gives access to configured options. Nested OptionSources are only
	// visible after the OptionSet has been completed.
	GetOption(name string) *ArbitraryOption

	VisitOptions(OptionVisitor)
}

// OptionGroup is a group set of options and option sources
// offering access to options
type OptionGroup interface {
	Options
	OptionSourceSource
}

// OptionSet is an OptionSource that can be used to add arbitrary options,
// either directly or by adding further nested OptionSource s.
type OptionSet interface {
	Options
	OptionSource
	OptionCompleter
	OptionEvaluator
	OptionSourceGroup

	AddStringOption(target *string, name, short, def string, desc string) *string
	AddStringArrayOption(target *[]string, name, short string, def []string, desc string) *[]string
	AddIntOption(target *int, name, short string, def int, desc string) *int
	AddUintOption(target *uint, name, short string, def uint, desc string) *uint
	AddBoolOption(target *bool, name, short string, def bool, desc string) *bool
	AddDurationOption(target *time.Duration, name, short string, def time.Duration, desc string) *time.Duration

	AddOption(otype OptionType, target interface{}, name, short string, def interface{}, desc string) interface{}
	AddRenamedOption(opt *ArbitraryOption, name, short string, desc string) interface{}

	// AddToFlags must complete the actual source if it contains nested
	// option sources (implements OptionSourceSource) to figure out all relevant
	// options to add. Afterwards the set should contain all options finally
	// added to the flag set (queryable by GetOption).
	AddToFlags(flags *pflag.FlagSet)
}

////////////////////////////////////////////////////////////////////////////////

type StringMapper func(string) string

func IdenityStringMapper(s string) string { return s }

////////////////////////////////////////////////////////////////////////////////

type OptionSetProxy func(otype OptionType, target interface{}, name, short string, def interface{}, desc string) interface{}

func (p OptionSetProxy) AddStringOption(target *string, name, short string, def string, desc string) *string {
	return p(StringOption, target, name, short, def, desc).(*string)
}
func (p OptionSetProxy) AddStringArrayOption(target *[]string, name, short string, def []string, desc string) *[]string {
	return p(StringArrayOption, target, name, short, def, desc).(*[]string)
}
func (p OptionSetProxy) AddIntOption(target *int, name, short string, def int, desc string) *int {
	return p(IntOption, target, name, short, def, desc).(*int)
}
func (p OptionSetProxy) AddUintOption(target *uint, name, short string, def uint, desc string) *uint {
	return p(UintOption, target, name, short, def, desc).(*uint)
}
func (p OptionSetProxy) AddBoolOption(target *bool, name, short string, def bool, desc string) *bool {
	return p(BoolOption, target, name, short, def, desc).(*bool)
}
func (p OptionSetProxy) AddDurationOption(target *time.Duration, name, short string, def time.Duration, desc string) *time.Duration {
	return p(DurationOption, target, name, short, def, desc).(*time.Duration)
}
func (p OptionSetProxy) AddOption(otype OptionType, target interface{}, name, short string, def interface{}, desc string) interface{} {
	return p(otype, target, name, short, def, desc)
}

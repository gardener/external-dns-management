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
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type ArbitraryOption struct {
	Name        string
	Description string
	Default     interface{}
	Target      interface{}
	Type        OptionType
	FlagSet     *pflag.FlagSet
}

func (this *ArbitraryOption) AddToCommand(cmd *cobra.Command) {
	this.FlagSet = cmd.PersistentFlags()
	this.Type.AddToFlags(this.FlagSet, nil, this.Name, "", this.Default, this.Description)
}

func (this *ArbitraryOption) Changed() bool {
	return this.FlagSet.Changed(this.Name)
}

func (this *ArbitraryOption) Value() interface{} {
	return reflect.ValueOf(this.Target).Elem().Interface()
}

func (this *ArbitraryOption) IsArray() bool {
	return strings.HasSuffix(this.Flag().Value.Type(), "Array")
}

func (this *ArbitraryOption) IsSlice() bool {
	return strings.HasSuffix(this.Flag().Value.Type(), "Slice")
}

func (this *ArbitraryOption) Flag() *pflag.Flag {
	return this.FlagSet.Lookup(this.Name)
}

func (this *ArbitraryOption) StringValue() string {
	v, _ := this.FlagSet.GetString(this.Name)
	return v
}
func (this *ArbitraryOption) StringArray() []string {
	v, _ := this.FlagSet.GetStringArray(this.Name)
	return v
}
func (this *ArbitraryOption) IntValue() int {
	v, _ := this.FlagSet.GetInt(this.Name)
	return v
}
func (this *ArbitraryOption) UintValue() uint {
	v, _ := this.FlagSet.GetUint(this.Name)
	return v
}
func (this *ArbitraryOption) BoolValue() bool {
	v, _ := this.FlagSet.GetBool(this.Name)
	return v
}
func (this *ArbitraryOption) DurationValue() time.Duration {
	v, _ := this.FlagSet.GetDuration(this.Name)
	return v
}

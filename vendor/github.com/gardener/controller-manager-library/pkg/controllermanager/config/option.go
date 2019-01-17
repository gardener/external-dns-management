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

package config

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"reflect"
)

type ArbitraryOption struct {
	Name        string
	Description string
	Default     interface{}
	Type        reflect.Type
	FlagSet     *pflag.FlagSet
}

func (this *ArbitraryOption) AddToCommand(cmd *cobra.Command) {
	this.FlagSet = cmd.PersistentFlags()
	switch this.Type {
	case reflect.TypeOf((*string)(nil)).Elem():
		this.FlagSet.String(this.Name, "", this.Description)
	case reflect.TypeOf(([]string)(nil)):
		this.FlagSet.StringArray(this.Name, nil, this.Description)
	case reflect.TypeOf((*int)(nil)).Elem():
		this.FlagSet.Int(this.Name, 0, this.Description)
	case reflect.TypeOf((*bool)(nil)).Elem():
		this.FlagSet.Bool(this.Name, false, this.Description)
	}
}

func (this *ArbitraryOption) Changed() bool {
	return this.FlagSet.Changed(this.Name)
}

func (this *ArbitraryOption) defaultAsValue() interface{} {
	if this.Default == nil {
		return nil
	}
	v := reflect.ValueOf(this.Default)
	if v.Kind() == reflect.Ptr {
		return v.Elem().Interface()
	}
	return this.Default
}

func (this *ArbitraryOption) StringValue() string {
	if this.FlagSet.Changed(this.Name) || this.Default == nil {
		v, _ := this.FlagSet.GetString(this.Name)
		return v
	}
	if this.Default != nil {
		return this.defaultAsValue().(string)
	}
	return ""
}
func (this *ArbitraryOption) StringArray() []string {
	if this.FlagSet.Changed(this.Name) || this.Default == nil {
		v, _ := this.FlagSet.GetStringArray(this.Name)
		return v
	}
	if this.Default != nil {
		return this.defaultAsValue().([]string)
	}
	return []string{}
}
func (this *ArbitraryOption) IntValue() int {
	if this.FlagSet.Changed(this.Name) || this.Default == nil {
		v, _ := this.FlagSet.GetInt(this.Name)
		return v
	}
	if this.Default != nil {
		return this.defaultAsValue().(int)
	}
	return 0
}
func (this *ArbitraryOption) BoolValue() bool {
	if this.FlagSet.Changed(this.Name) || this.Default == nil {
		v, _ := this.FlagSet.GetBool(this.Name)
		return v
	}
	if this.Default != nil {
		return this.defaultAsValue().(bool)
	}
	return false
}

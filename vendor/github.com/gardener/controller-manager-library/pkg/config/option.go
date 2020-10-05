/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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

/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package watches

import (
	"fmt"

	"github.com/Masterminds/semver/v3"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

// WatchConstraint is a match check for a dedicated watch context
type WatchConstraint interface {
	Check(ctx WatchContext) bool
}

// ConstraintFunction maps a check function to a constraint interface
type ConstraintFunction func(wctx WatchContext) bool

func (this ConstraintFunction) Check(wctx WatchContext) bool {
	return this(wctx)
}

// FlagOption check a bool option to be set
func FlagOption(name string) WatchConstraint {
	return NewFunctionWatchConstraint(
		func(wctx WatchContext) bool {
			b, err := wctx.GetBoolOption(name)
			return err == nil && b
		},
		fmt.Sprintf("flag %s", name),
	)
}

func StringOption(name string, values ...string) WatchConstraint {
	return NewFunctionWatchConstraint(
		func(wctx WatchContext) bool {
			s, err := wctx.GetStringOption(name)
			if err != nil {
				return false
			}
			for _, v := range values {
				if s == v {
					return true
				}
			}
			return false
		},
		fmt.Sprintf("option %s=%s", name, utils.Strings(values...)),
	)
}

// NewFunctionWatchConstraint creates a watch constraint using a `ConstraintFunction`
func NewFunctionWatchConstraint(f ConstraintFunction, desc string) WatchConstraint {
	return &constraintFunction{ConstraintFunction: f, desc: desc}
}

type constraintFunction struct {
	ConstraintFunction
	desc string
}

func (this constraintFunction) String() string {
	return this.desc
}

// Not negates a constraint
func Not(c WatchConstraint) WatchConstraint {
	return NewFunctionWatchConstraint(
		func(wctx WatchContext) bool {
			return !c.Check(wctx)
		},
		fmt.Sprintf("NOT(%s)", c),
	)
}

// And checks multiple constraints to be true
func And(c ...WatchConstraint) WatchConstraint {
	return NewFunctionWatchConstraint(
		func(wctx WatchContext) bool {
			for _, e := range c {
				if !e.Check(wctx) {
					return false
				}
			}
			return true
		},
		buildDescription("AND", c...),
	)
}

func buildDescription(op string, c ...WatchConstraint) string {
	desc := op + "("
	sep := ""
	for _, e := range c {
		desc = fmt.Sprintf("%s%s%s", desc, sep, e)
		sep = ", "
	}
	desc += ")"
	return desc
}

// Or checks multiple constraints to be not false
func Or(c ...WatchConstraint) WatchConstraint {
	return NewFunctionWatchConstraint(
		func(wctx WatchContext) bool {
			for _, e := range c {
				if e.Check(wctx) {
					return true
				}
			}
			return false
		},
		buildDescription("OR", c...),
	)
}

// APIServerVersion checks for a version constraint for the api server
func APIServerVersion(constraint string) WatchConstraint {
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		panic(err)
	}
	return NewFunctionWatchConstraint(
		func(wctx WatchContext) bool {
			return c.Check(wctx.Cluster().GetServerVersion())
		},
		fmt.Sprintf("(server version %s)", constraint),
	)
}

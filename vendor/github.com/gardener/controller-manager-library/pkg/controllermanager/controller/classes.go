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
 * limitations under the License.
 *
 */

package controller

import (
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

func ClassesFilter(classes *Classes) resources.ObjectFilter {
	return func(obj resources.Object) bool {
		return classes.IsResponsibleFor(nil, obj)
	}
}

type Classes struct {
	classes    utils.StringSet
	main       string
	def        string
	annotation string
}

func NewClassesByOption(c Interface, opt string, anno string, def string) *Classes {
	opt, _ = c.GetStringOption(opt)
	return NewClasses(c, opt, anno, def)
}

func NewClasses(logger logger.LogContext, value string, anno string, def string) *Classes {
	classes := newClasses(value, anno, def, def)
	if logger != nil {
		logger.Infof("responsible for classes: %s (%s)", classes.Main(), classes)
	}
	return classes
}

func NewTargetClassesByOption(c Interface, opt string, anno string, classes *Classes) *Classes {
	opt, _ = c.GetStringOption(opt)
	return NewTargetClasses(c, opt, anno, classes, classes.Default())
}

func NewTargetClasses(c Interface, value string, anno string, classes *Classes, def string) *Classes {
	if value == "" {
		if !classes.Contains(def) || classes.Main() != def {
			value = classes.Main()
		}
	}
	tclasses := newClasses(value, anno, def, def)
	if c != nil {
		c.Infof("target classes         : %s (%s)", tclasses.Main(), tclasses)
	}
	return tclasses
}

func newClasses(classes string, anno string, maindef, def string) *Classes {
	c := &Classes{classes: utils.StringSet{}, annotation: anno, def: def}
	if classes == "" {
		c.main = maindef
		c.classes.Add(c.main)
	} else {
		c.classes.AddAllSplitted(classes)
		index := strings.Index(classes, ",")
		if index < 0 {
			c.main = strings.ToLower(strings.TrimSpace(classes))
		} else {
			c.main = strings.ToLower(strings.TrimSpace(classes[:index]))
		}
	}
	return c
}

func (this *Classes) String() string {
	return this.classes.String()
}

func (this *Classes) IsDefault() bool {
	return this.main == this.def
}

func (this *Classes) Main() string {
	return this.main
}

func (this *Classes) Default() string {
	return this.def
}

func (this *Classes) Size() int {
	return len(this.classes)
}

func (this *Classes) Classes() utils.StringSet {
	return this.classes.Copy()
}

func (this *Classes) Contains(class string) bool {
	return this.classes.Contains(class)
}

func (this *Classes) IsResponsibleFor(logger logger.LogContext, obj resources.Object) bool {
	oclass := obj.GetAnnotations()[this.annotation]
	if oclass == "" {
		oclass = this.def
	}
	if !this.classes.Contains(oclass) {
		if logger != nil {
			logger.Debugf("%s: annotated dns class %q does not match specified class set %s -> skip ",
				obj.ObjectName(), oclass, this.classes)
		}
		return false
	}
	return true
}

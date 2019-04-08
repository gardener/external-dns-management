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

package utils

import (
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"strings"
)

const DEFAULT_CLASS = "gardendns"

const CLASS_ANNOTATION = "dns.gardener.cloud/class"

type Classes struct {
	classes utils.StringSet
	main    string
}

func NewClasses(classes string) *Classes {
	c := &Classes{classes: utils.StringSet{}}
	if classes == "" {
		c.main = DEFAULT_CLASS
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

func (this *Classes) Main() string {
	return this.main
}

func (this *Classes) Contains(class string) bool {
	return this.classes.Contains(class)
}

func (this *Classes) IsResponsibleFor(logger logger.LogContext, obj resources.Object) bool {
	oclass := obj.GetAnnotations()[CLASS_ANNOTATION]
	if oclass == "" {
		oclass = DEFAULT_CLASS
	}
	if !this.classes.Contains(oclass) {
		logger.Debugf("%s: annotated dns class %q does not match specified class set %s -> skip ",
			obj.ObjectName(), oclass, this.classes)
		return false
	}
	return true
}

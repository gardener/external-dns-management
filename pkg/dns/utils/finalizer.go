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
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

type Finalizer struct {
	name    string
	classes *Classes
}

func NewFinalizer(logger logger.LogContext, name string, classes *Classes) controller.Finalizer {
	n := finalizer(name, classes.Main())
	if n != name {
		logger.Infof("switching finalizer to %q", n)
	}
	if len(classes.classes) == 1 {
		return controller.NewDefaultFinalizer(n)
	}
	return &Finalizer{name, classes}

}

func finalizer(base string, class string) string {
	if class == DEFAULT_CLASS {
		return base
	}
	return class + "." + base
}

func (this *Finalizer) HasFinalizer(obj resources.Object) bool {
	for c := range this.classes.classes {
		if obj.HasFinalizer(finalizer(this.name, c)) {
			return true
		}
	}
	return false
}

func (this *Finalizer) SetFinalizer(obj resources.Object) error {
	err := obj.SetFinalizer(finalizer(this.name, this.classes.Main()))
	if err != nil {
		return err
	}
	for c := range this.classes.classes {
		if c != this.classes.Main() {
			err = obj.RemoveFinalizer(finalizer(this.name, c))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (this *Finalizer) RemoveFinalizer(obj resources.Object) error {
	for c := range this.classes.classes {
		err := obj.RemoveFinalizer(finalizer(this.name, c))
		if err != nil {
			return err
		}
	}
	return nil
}

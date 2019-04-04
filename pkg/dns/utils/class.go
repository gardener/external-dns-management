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
)

const DEFAULT_CLASS = "gardendns"

const CLASS_ANNOTATION = "dns.gardener.cloud/class"

func IsResponsibleFor(logger logger.LogContext, class string, obj resources.Object) bool {
	oclass := obj.GetAnnotations()[CLASS_ANNOTATION]
	if class == DEFAULT_CLASS && oclass == "" {
		return true
	}
	if class != oclass {
		logger.Debugf("%s: annotated dns class %q does not match specified class %q -> skip ",
			obj.ObjectName(), oclass, class)
		return false
	}
	return true
}

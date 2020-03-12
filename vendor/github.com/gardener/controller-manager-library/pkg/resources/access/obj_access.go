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

package access

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/errors"
)

const ANNOTATION_IGNORE_OWNERS = "resources.gardener.cloud/ignore-owners-for-access-control"

func CheckAccessWithRealms(object resources.Object, verb string, used resources.Object, rtypes RealmTypes) error {
	err := CheckAccess(object, verb, used)
	if err != nil {
		return err
	}
	if rtypes != nil {
		rtype := rtypes[verb]
		if rtype != nil {
			granted := rtype.RealmsForObject(used)
			if !granted.IsResponsibleFor(object) {
				return errors.New(errors.ERR_PERMISSION_DENIED, "permission denied by realms: %s <%s> %s",
					object.ClusterKey(), verb, used.ClusterKey())
			}
		}
	}
	return nil
}

func CheckAccess(object resources.Object, verb string, used resources.Object) error {
	var err error

	value, _ := resources.GetAnnotation(object.Data(), ANNOTATION_IGNORE_OWNERS)
	ignoreOwners := value == "true"
	owners := object.GetOwners()
	if !ignoreOwners && len(owners) > 0 {
		for o := range owners {
			ok, msg, aerr := Allowed(o, verb, used.ClusterKey())
			if !ok {
				if aerr != nil {
					err = fmt.Errorf("%s: %s: %s", o, msg, err)
				} else {
					err = fmt.Errorf("%s: %s", o, msg)
				}
			}
		}
	} else {
		o := object.ClusterKey()
		ok, msg, aerr := Allowed(o, "use", used.ClusterKey())
		if !ok {
			if aerr != nil {
				err = fmt.Errorf("%s: %s: %s", used.ClusterKey(), msg, err)
			} else {
				err = fmt.Errorf("%s: %s", used.ClusterKey(), msg)
			}
		}
	}
	return err
}

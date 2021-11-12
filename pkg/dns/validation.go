/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package dns

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

func ValidateDomainName(name string) error {
	check := NormalizeHostname(name)
	if strings.HasPrefix(check, "_") {
		// allow "_" prefix, as it is used for DNS challenges of Let's encrypt
		check = "x" + check[1:]
	}

	var errs []string
	if strings.HasPrefix(check, "*.") {
		errs = validation.IsWildcardDNS1123Subdomain(check)
	} else {
		errs = validation.IsDNS1123Subdomain(check)
	}

	if len(errs) > 0 {
		return fmt.Errorf("%q is no valid dns name (%v)", name, errs)
	}

	metaCheck := CalcMetaRecordDomainNameForValidation(check)
	if strings.HasPrefix(metaCheck, "*.") {
		errs = validation.IsWildcardDNS1123Subdomain(metaCheck)
	} else {
		errs = validation.IsDNS1123Subdomain(metaCheck)
	}
	if len(errs) > 0 {
		return fmt.Errorf("metadata record %q of %q is no valid dns name (%v)", metaCheck, name, errs)
	}

	labels := strings.Split(strings.TrimPrefix(check, "*."), ".")
	for i, label := range labels {
		if errs = validation.IsDNS1123Label(label); len(errs) > 0 {
			return fmt.Errorf("%d. label %q of %q is not valid (%v)", i+1, label, name, errs)
		}
	}
	metaLabels := strings.SplitN(strings.TrimPrefix(metaCheck, "*."), ".", 2)
	if errs = validation.IsDNS1123Label(metaLabels[0]); len(errs) > 0 {
		return fmt.Errorf("1. label %q of metadata record of %q is not valid (%v)", metaLabels[0], name, errs)
	}

	return nil
}

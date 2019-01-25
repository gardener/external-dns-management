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

package dns

import (
	api "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/controller-manager-library/pkg/resources"
)


func SupportedRecordType(t string) bool {
	switch t {
	case RS_CNAME, RS_A, RS_TXT:
		return true
	}
	return false
}

func DNSNameMatcher(dnsname string) resources.ObjectMatcher {
	return func(o resources.Object) bool {
		return o.Data().(*api.DNSEntry).Spec.DNSName == dnsname
	}
}

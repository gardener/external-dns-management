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

package source

import (
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"strconv"
	"strings"
)

func (this *sourceReconciler) exclude(dns string) bool {
	if this.excluded.Contains(dns) {
		return true
	}
	for d := range this.excluded {
		if strings.HasPrefix(d, "*.") {
			d = d[2:]
			i := strings.Index(dns, ".")
			if i >= 0 {
				if d == dns[i+1:] {
					return true
				}
			}
		}
	}
	return false
}

func (this *sourceReconciler) getDNSInfo(logger logger.LogContext, obj resources.Object, s DNSSource, current *DNSCurrentState) (*DNSInfo, error) {
	key := obj.GetAnnotations()[KEY_ANNOTATION]
	if key != this.key {
		logger.Infof("annotated key %q does not match specified key %q -> skip ", key, this.key)
		return nil, nil
	}
	a := obj.GetAnnotations()[DNS_ANNOTATION]
	current.AnnotatedNames = utils.StringSet{}
	for _, e := range strings.Split(a, ",") {
		e = strings.TrimSpace(e)
		if e != "" {
			current.AnnotatedNames.Add(e)
		}
	}
	info, err := s.GetDNSInfo(logger, obj, current)
	if err != nil {
		return info, err
	}
	for d := range info.Names {
		if this.exclude(d) {
			info.Names.Remove(d)
		}
	}
	if info.TTL == nil {
		a := obj.GetAnnotations()[TTL_ANNOTATION]
		if a != "" {
			ttl, err := strconv.ParseInt(a, 10, 64)
			if err != nil {
				return info, fmt.Errorf("invalid TTL: %s", err)
			}
			if ttl != 0 {
				info.TTL = &ttl
			}
		}
	}
	if info.Interval == nil {
		a := obj.GetAnnotations()[PERIOD_ANNOTATION]
		if a != "" {
			interval, err := strconv.ParseInt(a, 10, 64)
			if err != nil {
				return info, fmt.Errorf("invalid check Interval: %s", err)
			}
			if interval != 0 {
				info.Interval = &interval
			}
		}
	}
	return info, nil
}

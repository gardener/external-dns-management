// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
)

func (this *sourceReconciler) exclude(name dns.DNSSetName) bool {
	if this.excluded.Contains(name.DNSName) {
		return true
	}
	for d := range this.excluded {
		if strings.HasPrefix(d, "*.") {
			d = d[2:]
			i := strings.Index(name.DNSName, ".")
			if i >= 0 {
				if d == name.DNSName[i+1:] {
					return true
				}
			}
		}
	}
	return false
}

func (this *sourceReconciler) getDNSInfo(logger logger.LogContext, obj resources.Object, s DNSSource, current *DNSCurrentState) (*DNSInfo, bool, error) {
	obj = this.enrichAnnotations(logger, obj)

	if !this.classes.IsResponsibleFor(logger, obj) {
		return nil, false, nil
	}

	annos := obj.GetAnnotations()
	current.AnnotatedNames = utils.StringSet{}
	current.AnnotatedNames.AddAllSplittedSelected(annos[DNS_ANNOTATION], utils.StandardNonEmptyStringElement)
	current.AnnotatedRoutingPolicy = nil
	if a := annos[ROUTING_POLICY_ANNOTATION]; a != "" {
		policy := &v1alpha1.RoutingPolicy{}
		if err := json.Unmarshal([]byte(a), policy); err != nil {
			return nil, true, err
		}
		current.AnnotatedRoutingPolicy = policy
	}

	info, err := s.GetDNSInfo(logger, obj.Data(), current)
	if info != nil && info.Names != nil {
		for d := range info.Names {
			if this.exclude(d) {
				info.Names.Remove(d)
			}
		}
	}
	if err != nil {
		return info, true, err
	}
	if info == nil {
		return nil, true, nil
	}
	if info.TTL == nil {
		a := annos[TTL_ANNOTATION]
		if a != "" {
			ttl, err := strconv.ParseInt(a, 10, 64)
			if err != nil {
				return info, true, fmt.Errorf("invalid TTL: %s", err)
			}
			if ttl != 0 {
				info.TTL = &ttl
			}
		}
	}
	if info.Interval == nil {
		a := annos[PERIOD_ANNOTATION]
		if a != "" {
			interval, err := strconv.ParseInt(a, 10, 64)
			if err != nil {
				return info, true, fmt.Errorf("invalid check Interval: %s", err)
			}
			if interval != 0 {
				info.Interval = &interval
			}
		}
	}
	if info.RoutingPolicy == nil {
		info.RoutingPolicy = current.AnnotatedRoutingPolicy
	}
	return info, true, nil
}

func (this *sourceReconciler) enrichAnnotations(logger logger.LogContext, obj resources.Object) resources.Object {
	addons := this.annotations.GetInfoFor(obj.ClusterKey())
	if len(addons) > 0 {
		obj = obj.DeepCopy()
		annos := getSafeMap(obj.GetAnnotations())

		annotatedNames := utils.StringSet{}
		annotatedNames.AddAllSplittedSelected(annos[DNS_ANNOTATION], utils.StandardNonEmptyStringElement)

		for k, v := range addons {
			if k == DNS_ANNOTATION {
				annotatedNames.AddAllSplittedSelected(v, utils.StandardNonEmptyStringElement)
				logger.Infof("adding dns names by annotation injection: %s", v)
			} else {
				if old, ok := annos[k]; !ok || old != v {
					annos[k] = v
					logger.Infof("using annotation injection: %s=%s", k, v)
				}
			}
		}

		if len(annotatedNames) > 0 {
			annos[DNS_ANNOTATION] = strings.Join(annotatedNames.AsArray(), ",")
		}
		obj.SetAnnotations(annos)
	}
	return obj
}

func getSafeMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

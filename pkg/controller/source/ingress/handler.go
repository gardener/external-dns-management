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

package ingress

import (
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"github.com/gardener/external-dns-management/pkg/dns/source"
	api "k8s.io/api/extensions/v1beta1"
)

type IngressSource struct {
	source.DefaultDNSSource
}

func NewIngressSource(controller.Interface) (source.DNSSource, error) {
	return &IngressSource{DefaultDNSSource: source.DefaultDNSSource{Events: map[resources.ClusterObjectKey]map[string]string{}}}, nil
}

func (this *IngressSource) GetDNSInfo(logger logger.LogContext, obj resources.Object, current *source.DNSCurrentState) (*source.DNSInfo, error) {
	events := this.GetEvents(obj.ClusterKey())
	info := &source.DNSInfo{Targets: this.GetTargets(obj), Feedback: source.NewEventFeedback(logger, obj, events)}
	data := obj.Data().(*api.Ingress)
	info.Names = utils.StringSet{}
	all := current.AnnotatedNames.Contains("all")
	for _, i := range data.Spec.Rules {
		if i.Host != "" && (all || current.AnnotatedNames.Contains(i.Host)) {
			info.Names.Add(i.Host)
		}
	}
	_, del := current.AnnotatedNames.DiffFrom(info.Names)
	del.Remove("all")
	if len(del) > 0 {
		return info, fmt.Errorf("annotated dns names %s not declared by ingress", del)
	}
	return info, nil
}

func (this *IngressSource) GetTargets(obj resources.Object) utils.StringSet {
	ing := obj.Data().(*api.Ingress)
	set := utils.StringSet{}
	for _, i := range ing.Status.LoadBalancer.Ingress {
		if i.Hostname != "" && i.IP == "" {
			set.Add(i.Hostname)
		} else {
			if i.IP != "" {
				set.Add(i.IP)
			}
		}
	}
	return set
}

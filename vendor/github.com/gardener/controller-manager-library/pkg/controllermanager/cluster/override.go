/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package cluster

import (
	restclient "k8s.io/client-go/rest"
)

type APIServerOverride struct{}

var _ Extension = &APIServerOverride{}
var _ RestConfigExtension = &APIServerOverride{}

func (this *APIServerOverride) ExtendConfig(def Definition, cfg *Config) {
	cfg.AddStringOption(nil, "apiserver-override", "", "", "replace api server url from kubeconfig")
}

func (this *APIServerOverride) Extend(cluster Interface, cfg *Config) error {
	return nil
}

func (this *APIServerOverride) TweakRestConfig(def Definition, cfg *Config, restcfg *restclient.Config) error {
	opt := cfg.GetOption("apiserver-override")
	if opt != nil {
		if opt.StringValue() != "" {
			restcfg.Host = opt.StringValue()
		}
	}
	return nil
}

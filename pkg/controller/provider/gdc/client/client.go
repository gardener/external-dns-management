// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Package client provides a common function to authenticate and instantiate a
// Kubernetes client for GDC.
package client

import (
	"encoding/base64"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/auth"
)

// Get returns a Kubernetes client to the specified cluster of GDC.
func Get(
	config *OrgClusterConfig,
	serviceAccount *auth.ServiceAccount,
	scheme *runtime.Scheme,
) (client.WithWatch, error) {
	restConfig, err := getRESTConfig(config, serviceAccount)
	if err != nil {
		return nil, fmt.Errorf("retrieving Kubernetes REST configuration: %w", err)
	}

	c, err := client.NewWithWatch(restConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client from config: %w", err)
	}

	return c, nil
}

func getRESTConfig(config *OrgClusterConfig, serviceAccount *auth.ServiceAccount) (*rest.Config, error) {
	certData, err := base64.StdEncoding.DecodeString(config.CAData)
	if err != nil {
		return nil, fmt.Errorf("decode cadata: %w", err)
	}
	audience := config.OrgClusterURL

	stsTS := auth.NewCachedSTSTokenSource(audience, serviceAccount, auth.WithCACert(certData))
	cfg := &rest.Config{
		Host: config.OrgClusterURL,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: certData,
		},
	}

	// Injects STS tokens as the bearer authorization header in the requests.
	cfg.Wrap(transport.TokenSourceWrapTransport(stsTS))

	return cfg, nil
}

// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

type InfobloxConfig struct {
	Host            *string            `json:"host,omitempty"`
	Port            *int               `json:"port,omitempty"`
	SSLVerify       *bool              `json:"sslVerify,omitempty"`
	Version         *string            `json:"version,omitempty"`
	View            *string            `json:"view,omitempty"`
	PoolConnections *int               `json:"httpPoolConnections,omitempty"`
	RequestTimeout  *int               `json:"httpRequestTimeout,omitempty"`
	CaCert          *string            `json:"caCert,omitempty"`
	MaxResults      int                `json:"maxResults,omitempty"`
	ProxyURL        *string            `json:"proxyUrl,omitempty"`
	ExtAttrs        *map[string]string `json:"extAttrs,omitempty"`
}

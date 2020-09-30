/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package certmgmt

import (
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
)

type Config struct {
	CommonName        string
	Organization      []string
	Hosts             CertificateHosts
	Validity          time.Duration
	Rest              time.Duration
	ExternallyManaged bool
}

type CertificateInfo interface {
	Cert() []byte
	Key() []byte
	CACert() []byte
	CAKey() []byte
}

type CertificateAccess interface {
	Get(logger.LogContext) (CertificateInfo, error)
	Set(logger.LogContext, CertificateInfo) error
}

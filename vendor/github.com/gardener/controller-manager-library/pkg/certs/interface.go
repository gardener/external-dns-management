/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package certs

import (
	"crypto/tls"

	"github.com/gardener/controller-manager-library/pkg/certmgmt"
)

type CertificateSource interface {
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
	GetCertificateInfo() certmgmt.CertificateInfo
}

type CertificateConsumerUpdater interface {
	UpdateCertificate(info certmgmt.CertificateInfo)
}

type CertificateWatch interface {
	RegisterConsumer(h CertificateConsumerUpdater)
}

type CertificateUpdaterFunc func(info certmgmt.CertificateInfo)

func (this CertificateUpdaterFunc) UpdateCertificate(info certmgmt.CertificateInfo) {
	this(info)
}

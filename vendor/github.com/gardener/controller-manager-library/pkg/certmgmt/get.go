/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package certmgmt

import (
	"crypto/tls"
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
)

func GetCertificateInfo(logger logger.LogContext, access CertificateAccess, cfg *Config) (CertificateInfo, error) {
	r, err := access.Get(logger)
	if err != nil {
		return nil, fmt.Errorf("error reading from certificate access: %s", err)
	}
	r, err = UpdateCertificate(r, cfg)
	if err != nil {
		return nil, fmt.Errorf("certmgmt update failed: %s", err)
	}

	err = access.Set(logger, r)
	if err != nil {
		return r, fmt.Errorf("certificate update failed: %s", err)
	}
	return r, nil
}

func GetCertificate(info CertificateInfo) (tls.Certificate, error) {
	return tls.X509KeyPair(info.Cert(), info.Key())
}

/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved.
 * This file is licensed under the Apache Software License, v. 2 except as noted
 * otherwise in the LICENSE file
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

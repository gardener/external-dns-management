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
	"io/ioutil"
)

func LoadCertInfo(certFile, keyFile, caFile, cakeyFile string) (CertificateInfo, error) {
	certPEMBlock, err := ioutil.ReadFile(certFile)
	if err != nil {
		return NewCertInfo(nil, nil, nil, nil), err
	}
	keyPEMBlock, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return NewCertInfo(certPEMBlock, nil, nil, nil), err
	}

	var caPEMBlock []byte
	if caFile != "" {
		caPEMBlock, err = ioutil.ReadFile(caFile)
		if err != nil {
			return NewCertInfo(certPEMBlock, keyPEMBlock, nil, nil), err
		}
	}
	var cakeyPEMBlock []byte
	if cakeyFile != "" {
		cakeyPEMBlock, err = ioutil.ReadFile(cakeyFile)
		if err != nil {
			return NewCertInfo(certPEMBlock, keyPEMBlock, caPEMBlock, nil), err
		}
	}
	return NewCertInfo(certPEMBlock, keyPEMBlock, caPEMBlock, cakeyPEMBlock), err
}

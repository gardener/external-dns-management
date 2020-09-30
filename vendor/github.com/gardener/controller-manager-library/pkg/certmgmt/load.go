/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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

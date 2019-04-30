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

package integration

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	_ "github.com/gardener/external-dns-management/pkg/controller/provider/mock/controller"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/ingress"
	_ "github.com/gardener/external-dns-management/pkg/controller/source/service"
)

var testEnv *TestEnv

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	var err error

	kubeconfig := os.Getenv("KUBECONFIG")
	Ω(kubeconfig).ShouldNot(Equal(""))

	args := []string{
		"--kubeconfig", kubeconfig,
		"--identifier", "integrationtest",
		"--controllers", "mock-inmemory,dnssources",
		"--omit-lease",
		"--reschedule-delay", "15s",
		"--pool.size", "10",
	}
	go runControllerManager(args)

	testEnv, err = NewTestEnv(kubeconfig, "test")
	Ω(err).Should(BeNil())
})

var _ = AfterSuite(func() {
	if testEnv != nil {
		testEnv.Infof("AfterSuite")
	}
})

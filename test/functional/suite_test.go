// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package functional

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = BeforeSuite(func() {
})

func TestFunctionalTests(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Functional Test Suite for DNS Controller Manager")
}

var _ = AfterSuite(func() {
})

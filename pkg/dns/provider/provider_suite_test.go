// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"testing"

	cmllogger "github.com/gardener/controller-manager-library/pkg/logger"
	ginkgov2 "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUtilsSuite(t *testing.T) {
	RegisterFailHandler(ginkgov2.Fail)
	cmllogger.SetOutput(ginkgov2.GinkgoWriter)
	ginkgov2.RunSpecs(t, "Provider Suite")
}

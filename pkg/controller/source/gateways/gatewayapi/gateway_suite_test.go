/*
 * SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 *
 */

package gatewayapi_test

import (
	"testing"

	ginkgov2 "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestUtilsSuite(t *testing.T) {
	RegisterFailHandler(ginkgov2.Fail)
	ginkgov2.RunSpecs(t, "Networking Kubernetes Gateway Suite")
}

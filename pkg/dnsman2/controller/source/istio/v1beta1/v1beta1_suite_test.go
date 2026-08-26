// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package v1beta1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestV1Beta1(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Istio v1beta1 Suite")
}

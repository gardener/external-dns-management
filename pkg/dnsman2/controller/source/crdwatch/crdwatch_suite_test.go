// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package crdwatch_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCRDWatch(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRD watch Suite")
}

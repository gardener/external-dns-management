// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package google_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGoogle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Google Cloud Test Suite")
}

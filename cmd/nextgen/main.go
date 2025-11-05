// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"

	"github.com/gardener/gardener/cmd/utils"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/gardener/external-dns-management/cmd/nextgen/app"
)

func main() {
	utils.DeduplicateWarnings()

	if err := app.NewCommand().ExecuteContext(signals.SetupSignalHandler()); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

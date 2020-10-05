/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package run

import (
	"context"
	"log"
	"os"
	"runtime/pprof"

	"github.com/gardener/controller-manager-library/pkg/configmain"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
)

type Runner func() error

func Run(ctx context.Context, runner Runner) error {
	mcfg := configmain.Get(ctx)
	err := mcfg.Evaluate()
	if err != nil {
		return err
	}
	cfg := GetConfig(mcfg)

	if cfg.CPUProfile != "" {
		f, err := os.Create(cfg.CPUProfile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}
	if cfg.LogLevel != "" {
		err = logger.SetLevel(cfg.LogLevel)
	}
	if err == nil {
		err = runner()
	}
	if err != nil {
		ctxutil.Cancel(ctx)
	}
	return err
}

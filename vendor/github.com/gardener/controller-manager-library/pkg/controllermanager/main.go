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

package controllermanager

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/spf13/cobra"
)

func Start(use, short, long string) {
	args := strings.Split(use, " ")
	Configure(args[0], long).ByDefault().Start(use, short)
}

func (this Configuration) Start(use, short string) {
	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	def := this.Definition()
	long := def.GetDescription()
	var (
		ctx = ctxutil.CancelContext(ctxutil.SyncContext(context.Background()))
		c   = make(chan os.Signal, 2)
	)

	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-c
		logger.Infof("process is being terminated")
		ctxutil.Cancel(ctx)
		<-c
		logger.Infof("process is aborted immediately")
		os.Exit(0)
	}()

	//	if err := plugins.HandleCommandLine("--plugin-dir", os.Args); err != nil {
	//		panic(err)
	//	}

	command := NewCommand(ctx, use, short, long, def)
	if err := command.Execute(); err != nil {
		os.Exit(1)
	}

	var gracePeriodSeconds time.Duration = 120
	logger.Infof("waiting for everything to shutdown (max. %d seconds)", gracePeriodSeconds)
	ctxutil.SyncPointWait(ctx, gracePeriodSeconds*time.Second)
	logger.Infof("%s exits.", use)
}

func NewCommand(ctx context.Context, use, short, long string, def *Definition) *cobra.Command {
	var (
		cmd = &cobra.Command{
			Use:   use,
			Short: short,
			Long:  long,
			RunE: func(c *cobra.Command, args []string) error {
				if err := run(ctx, def); err != nil {
					fmt.Fprintf(os.Stderr, "%s\n", err)
					os.Exit(1)
				}
				return nil
			},
		}
		cfg = config.NewConfig()
	)
	def.ExtendConfig(cfg)
	cfg.AddToCommand(cmd)
	ctx = config.WithConfig(ctx, cfg)

	return cmd
}

func run(ctx context.Context, def *Definition) error {
	var err error
	var controllerManager *ControllerManager

	logger.Infof("starting controller manager")

	cfg := config.Get(ctx)
	if cfg.LogLevel != "" {
		err = logger.SetLevel(cfg.LogLevel)
	}
	if err == nil {
		controllerManager, err = NewControllerManager(ctx, def)
	}
	if err != nil {
		ctxutil.Cancel(ctx)
		return err
	}

	return controllerManager.Run()
}

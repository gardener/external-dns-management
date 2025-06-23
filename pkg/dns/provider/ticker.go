// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
)

type Ticker struct {
	ticker func()
}

func NewTicker(ticker func()) *Ticker {
	return &Ticker{ticker: ticker}
}

func (t *Ticker) TickWhile(log logger.LogContext, f func()) {
	if t != nil {
		nextWarn := time.Now().Add(10 * time.Minute)
		privateTicker := time.NewTicker(30 * time.Second)
		done := make(chan struct{})
		go func() {
			for {
				select {
				case <-privateTicker.C:
					t.ticker()
					if time.Now().After(nextWarn) {
						nextWarn = time.Now().Add(10 * time.Minute)
						log.Warn("operation takes more than 10 minutes")
					}
				case <-done:
					privateTicker.Stop()
					return
				}
			}
		}()
		defer close(done)
	}
	f()
}

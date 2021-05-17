/*
 * Copyright 2021 Mandelsoft. All rights reserved.
 *  This file is licensed under the Apache Software License, v. 2 except as noted
 *  otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package utils

import (
	"fmt"
)

// NotificationLogger is a common super interface for
// a logger.LogContext and a Notifier
type NotificationLogger interface {
	Infof(msg string, args ...interface{})
	Info(args ...interface{})
	Debugf(msg string, args ...interface{})
	Debug(args ...interface{})

	Warnf(msg string, args ...interface{})
	Warn(args ...interface{})
	Errorf(msg string, args ...interface{})
	Error(args ...interface{})
}

// Notifier is a conditional notification controller
// It stores informational messages added with Add(false,...) or a Debug method
// if it is not active.
// Once activated via Add(true,...) or a direct call to
// a NotificationLogger Info/Warn/Error method, first all the pending
// messages are echoed. All succeeding informational
// message are echoed, once it is activated.
type Notifier struct {
	logger  NotificationLogger
	parent  *Notifier
	pending []string
	active  bool
}

var _ NotificationLogger = &Notifier{}

// NewNotifier creates a new notification controller
// for a NotificationLogger. Notifiers can be cascaded
// if a Notifier is passed as NotificationLogger.
// If a nested Notifier is activated all upstream
// Notifiers are activated, also.
// But an unactivated nested Notifier does not
// influence the stored upstream messages. Once
// the nested one is discarded without activation, its
// stored messages are discarded.
// A new Notifier, cascaded or not, always starts
// in state inactive. This can be used to start
// sub sections, that can completely discarded
// if no important message occurs.
func NewNotifier(log NotificationLogger, headers ...string) *Notifier {
	if len(headers) == 1 && headers[0] == "" {
		headers = nil
	}
	var parent *Notifier
	if n, ok := log.(*Notifier); ok {
		parent = n
	}
	return &Notifier{
		logger:  log,
		pending: headers,
		parent:  parent,
	}
}

func (this *Notifier) Activate() {
	if !this.active {
		if this.parent != nil {
			this.parent.Activate()
		}
		if len(this.pending) > 0 {
			for _, p := range this.pending {
				this.logger.Info(p)
			}
			this.pending = nil
		}
		this.active = true
	}
}

func (this *Notifier) Infof(msg string, args ...interface{}) {
	this.Add(true, msg, args...)
}

func (this *Notifier) Info(args ...interface{}) {
	msg := fmt.Sprint(args...)
	this.Add(true, "%s", msg)
}

func (this *Notifier) Debugf(msg string, args ...interface{}) {
	this.Add(false, msg, args...)
}

func (this *Notifier) Debug(args ...interface{}) {
	msg := fmt.Sprint(args...)
	this.Add(false, "%s", msg)
}

func (this *Notifier) Warnf(msg string, args ...interface{}) {
	this.Activate()
	this.logger.Warnf(msg, args)
}

func (this *Notifier) Warn(args ...interface{}) {
	msg := fmt.Sprint(args...)
	this.Activate()
	this.logger.Warn(msg, args)
}

func (this *Notifier) Errorf(msg string, args ...interface{}) {
	this.Activate()
	this.logger.Errorf(msg, args)
}

func (this *Notifier) Error(args ...interface{}) {
	msg := fmt.Sprint(args...)
	this.Activate()
	this.logger.Error(msg, args)
}

func (this *Notifier) Add(print bool, msg string, args ...interface{}) {
	if print || this.active {
		if len(this.pending) > 0 {
			for _, p := range this.pending {
				this.logger.Info(p)
			}
			this.pending = nil
		}
		this.logger.Infof(msg, args...)
		this.active = true
	} else {
		this.pending = append(this.pending, fmt.Sprintf(msg, args...))
	}
}

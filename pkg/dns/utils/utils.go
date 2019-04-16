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

package utils

import (
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
	"strings"
)

func Match(hostname, domain string) bool {
	return strings.HasSuffix(hostname, "."+domain) || domain == hostname
}

func MatchSet(hostname string, domains utils.StringSet) int {
	length := 0
	for d := range domains {
		if len(d) > length && Match(hostname, d) {
			length = len(d)
		}
	}
	return length
}

type LogMessage struct {
	msg    string
	logged bool
}

func NewLogMessage(msg string, args ...interface{}) *LogMessage {
	return &LogMessage{msg: fmt.Sprintf(msg, args...)}
}

func (this *LogMessage) Get() string {
	return this.msg
}

func (this *LogMessage) out(out func(string, ...interface{}), args ...interface{}) bool {
	if !this.logged {
		msgfmt := "%s"
		msgargs := []interface{}{this.msg}
		if len(args) > 0 {
			msgfmt += ", " + fmt.Sprintf("%s", args[0])
			msgargs = append(msgargs, args[1:]...)
		}
		out(msgfmt, msgargs...)
		this.logged = true
		return true
	}
	if len(args) > 0 {
		out(fmt.Sprintf("%s", args[0]), args[1:]...)
	}
	return false
}

func (this *LogMessage) Infof(logger logger.LogContext, add ...interface{}) bool {
	return this.out(logger.Infof, add...)
}

func (this *LogMessage) Errorf(logger logger.LogContext, add ...interface{}) bool {
	return this.out(logger.Errorf, add...)
}

func (this *LogMessage) Warnf(logger logger.LogContext, add ...interface{}) bool {
	return this.out(logger.Warnf, add...)
}

func (this *LogMessage) Debugf(logger logger.LogContext, add ...interface{}) bool {
	return this.out(logger.Debugf, add...)
}

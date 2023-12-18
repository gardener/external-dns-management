/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package remote

import (
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/server/remote/common"
)

type memoryLogger struct {
	logger  logger.LogContext
	entries []*common.LogEntry
}

var _ logger.LogContext = &memoryLogger{}

func newMemoryLogger(logger logger.LogContext) *memoryLogger {
	return &memoryLogger{logger: logger}
}

func (l *memoryLogger) NewContext(_, _ string) logger.LogContext {
	panic("unsupported")
}

func (l *memoryLogger) AddIndent(_ string) logger.LogContext {
	panic("unsupported")
}

func (l *memoryLogger) Info(msg ...interface{}) {
	l.logger.Info(msg...)
	l.addEntry(common.LogEntry_INFO, fmt.Sprintf("%s", msg...))
}

func (l *memoryLogger) Debug(msg ...interface{}) {
	l.logger.Debug(msg...)
	l.addEntry(common.LogEntry_DEBUG, fmt.Sprintf("%s", msg...))
}

func (l *memoryLogger) Warn(msg ...interface{}) {
	l.logger.Warn(msg...)
	l.addEntry(common.LogEntry_WARN, fmt.Sprintf("%s", msg...))
}

func (l *memoryLogger) Error(msg ...interface{}) {
	l.logger.Error(msg...)
	l.addEntry(common.LogEntry_ERROR, fmt.Sprintf("%s", msg...))
}

func (l *memoryLogger) Infof(msgfmt string, args ...interface{}) {
	l.logger.Infof(msgfmt, args...)
	l.addEntry(common.LogEntry_INFO, fmt.Sprintf(msgfmt, args...))
}

func (l *memoryLogger) Debugf(msgfmt string, args ...interface{}) {
	l.logger.Debugf(msgfmt, args...)
	l.addEntry(common.LogEntry_DEBUG, fmt.Sprintf(msgfmt, args...))
}

func (l *memoryLogger) Warnf(msgfmt string, args ...interface{}) {
	l.logger.Warnf(msgfmt, args...)
	l.addEntry(common.LogEntry_WARN, fmt.Sprintf(msgfmt, args...))
}

func (l *memoryLogger) Errorf(msgfmt string, args ...interface{}) {
	l.logger.Errorf(msgfmt, args...)
	l.addEntry(common.LogEntry_ERROR, fmt.Sprintf(msgfmt, args...))
}

func (l *memoryLogger) addEntry(level common.LogEntry_Level, msg string) {
	l.entries = append(l.entries, &common.LogEntry{
		Timestamp: time.Now().UnixNano(),
		Level:     level,
		Message:   msg,
	})
}

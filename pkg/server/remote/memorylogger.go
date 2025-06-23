// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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

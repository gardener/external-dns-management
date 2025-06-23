// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// SetLastUpdateTime sets the time wrapper to the current UTC time.
func SetLastUpdateTime(lastUpdateTime **metav1.Time) {
	*lastUpdateTime = &metav1.Time{Time: time.Now().UTC()}
}

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

package reconcile

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

func Succeeded(logger logger.LogContext, msg ...interface{}) Status {
	if len(msg) > 0 {
		logger.Info(msg...)
	}
	return Status{true, nil, -1}
}

func Repeat(logger logger.LogContext, err ...error) Status {
	for _, e := range err {
		logger.Error(e)
	}
	return Status{false, nil, -1}
}

func RepeatOnError(logger logger.LogContext, err error) Status {
	if err == nil {
		return Succeeded(logger)
	}
	return Repeat(logger, err)
}

func Delay(logger logger.LogContext, err error) Status {
	if err == nil {
		err = fmt.Errorf("reconcilation with problem")
	} else {
		logger.Warn(err)
	}
	return Status{true, err, -1}
}

func DelayOnError(logger logger.LogContext, err error, ratelimiter ...utils.RateLimiter) Status {
	if err == nil {
		for _, r := range ratelimiter {
			r.Succeeded()
		}
		return Succeeded(logger)
	}
	delay := time.Duration(-1)
	for _, r := range ratelimiter {
		r.Failed()
		l := r.RateLimit()
		if delay < 0 || l < delay {
			delay = l
		}
	}
	logger.Warn(err)
	return Status{true, err, delay}
}

func DelayOnErrorOrReschedule(logger logger.LogContext, err error, d time.Duration) Status {
	if err == nil {
		return Succeeded(logger).RescheduleAfter(d)
	}
	return Delay(logger, err)
}

func RescheduleAfter(logger logger.LogContext, d time.Duration) Status {
	return Succeeded(logger).RescheduleAfter(d)
}

func Failed(logger logger.LogContext, err error) Status {
	logger.Error(err)
	return Status{false, err, -1}
}

func Recheck(logger logger.LogContext, err error, interval ...time.Duration) Status {
	if err != nil {
		logger.Error(err)
	}
	i := 30 * time.Minute
	if len(interval) > 0 {
		i = interval[0]
	}
	return Status{err == nil, err, i}
}

func FailedOnError(logger logger.LogContext, err error) Status {
	if err == nil {
		return Succeeded(logger)
	}
	return Failed(logger, err)
}

func FinalUpdate(logger logger.LogContext, modified bool, obj resources.Object) Status {
	if modified {
		err := obj.Update()
		if err != nil {
			if errors.IsConflict(err) {
				return Repeat(logger, err)
			}
		}
		return DelayOnError(logger, err)
	}
	return Succeeded(logger)
}

func UpdateStatus(logger logger.LogContext, upd resources.ObjectStatusUpdater, d ...time.Duration) Status {
	err := upd.UpdateStatus()
	if err != nil {
		return Delay(logger, err)
	}
	if len(d) == 0 {
		return Succeeded(logger)
	}
	return RescheduleAfter(logger, d[0])
}

func Update(logger logger.LogContext, upd resources.ObjectUpdater, d ...time.Duration) Status {
	err := upd.Update()
	if err != nil {
		return Delay(logger, err)
	}
	if len(d) == 0 {
		return Succeeded(logger)
	}
	return RescheduleAfter(logger, d[0])
}

////////////////////////////////////////////////////////////////////////////////

func StringEqual(field *string, val string) bool {
	if field == nil {
		return val == ""
	}
	return val == *field
}

func StringValue(field *string) string {
	if field == nil {
		return ""
	}
	return *field
}

func StringSet(field **string, val string) {
	if val == "" {
		*field = nil
	}
	*field = &val
}

////////////////////////////////////////////////////////////////////////////////

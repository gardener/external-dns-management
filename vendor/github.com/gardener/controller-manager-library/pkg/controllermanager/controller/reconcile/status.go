/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package reconcile

import "time"

func (this Status) IsSucceeded() bool {
	return this.Completed && this.Error == nil
}

func (this Status) IsDelayed() bool {
	return this.Completed && this.Error != nil
}

func (this Status) IsFailed() bool {
	return !this.Completed && this.Error != nil
}

func (this Status) MustBeRepeated() bool {
	return !this.Completed && this.Error == nil
}

func (this Status) RescheduleAfter(d time.Duration) Status {
	if this.Interval < 0 || d < this.Interval {
		this.Interval = d
	}
	return this
}

func (this Status) Stop() Status {
	this.Interval = 0
	return this
}

func (this Status) StopIfSucceeded() Status {
	if this.IsSucceeded() {
		this.Interval = 0
	}
	return this
}

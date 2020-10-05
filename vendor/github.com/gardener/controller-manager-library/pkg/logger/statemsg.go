/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 *
 */

package logger

type OptionalSingletonMessage struct {
	function FormattingFunction
	msg      string
	args     []interface{}
	done     bool
}

// NewOptionalSingletonMessage creates a new message outputer with a singleton
// section/header message that is printed before the first regular output,
// if there is such an output
func NewOptionalSingletonMessage(function FormattingFunction, msg string, args ...interface{}) *OptionalSingletonMessage {
	return &OptionalSingletonMessage{function, msg, args, false}
}

// Once outputs the configured message the first time it is called
func (this *OptionalSingletonMessage) Once() {
	this.Default(this.msg, this.args...)
}

// Out outputs a message after calling Once to ensure a header/section message
func (this *OptionalSingletonMessage) Out(msg string, args ...interface{}) {
	this.Once()
	this.function(msg, args...)
}

// Default outputs a given default message if Once has never been called and disables
// the standard Once message
func (this *OptionalSingletonMessage) Default(msg string, args ...interface{}) {
	if !this.done {
		this.function(msg, args...)
		this.done = true
	}
}

// Enforce always outputs the given message and omits further default output
// (explicit via method Default or implicit via method Once)
// without checking whether this is the first call
func (this *OptionalSingletonMessage) Enforce(msg string, args ...interface{}) {
	this.Reset()
	this.Default(msg, args...)
}

// ResetWith restarts the object with a new message
func (this *OptionalSingletonMessage) ResetWith(msg string, args ...interface{}) {
	this.msg = msg
	this.args = args
	this.Reset()
}

// Reset resets the done flag (
func (this *OptionalSingletonMessage) Reset() {
	this.done = false
}

// IsPending returns whether the header/section message is still pending
func (this *OptionalSingletonMessage) IsPending() bool {
	return !this.done
}

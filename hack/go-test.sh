#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2023 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

echo go test $@
go test $@ | grep -v 'no test files'
exit "${PIPESTATUS[0]}"
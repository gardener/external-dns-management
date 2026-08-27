#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

set -e
set -u
set -o pipefail

go test $@ | grep -v 'no test files'

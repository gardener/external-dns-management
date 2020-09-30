#!/usr/bin/env bash

# SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

# run something in a directory
# goes hand-in .run-controller-gen.sh

cd "$1"
echo "DIR: $1"
shift
echo "CMD: $@"
"$@"

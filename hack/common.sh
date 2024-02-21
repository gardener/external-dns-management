#!/bin/bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0
set -e

DIRNAME="$(echo "$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )")"
ROOT="$DIRNAME/.."

# Enable tracing in this script off by setting the TRACE variable in your
# environment to any value:
#
# $ TRACE=1 test.sh
TRACE=${TRACE:-""}
if [[ -n "$TRACE" ]]; then
  set -x
fi

# Turn colors in this script off by setting the NO_COLOR variable in your
# environment to any value:
#
# $ NO_COLOR=1 test.sh
NO_COLOR=${NO_COLOR:-""}
if [[ -z "$NO_COLOR" ]]; then
  header=$'\e[1;33m'
  reset=$'\e[0m'
else
  header=''
  reset=''
fi

function header_text {
  echo "$header$*$reset"
}

SOURCE_TREES=(./pkg/... ./controllers/...)
CMD_TREES=(./controllers/...)

VERSIONFILE_VERSION="$(cat "$DIRNAME/../VERSION")"
VERSION="${VERSION:-${EFFECTIVE_VERSION:-"$VERSIONFILE_VERSION"}}"

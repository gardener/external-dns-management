#!/usr/bin/env bash

# This builds and runs controller-gen in a particular context
# it's the equivalent of `go run sigs.k8s.io/controller-tools/cmd/controller-gen`
# if you could somehow do that without modifying your go.mod.

current_dir="$(pwd)"
if ! readlink -f . &>/dev/null; then
    echo "you're probably on OSX.  Please install gnu readlink -- otherwise you're missing the most useful readlink flag."
    exit 1
fi
tool_dir="$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")"
if [ -d "${tool_dir}/../vendor" ]; then
  cd "${tool_dir}/../vendor"
else
   cd "${tool_dir}/../../../.."
fi
cd "sigs.k8s.io/controller-tools"
chmod a+x "${tool_dir}/run-in.sh"
go run -v -exec "${tool_dir}/run-in.sh ${current_dir} " ./cmd/controller-gen "$@"

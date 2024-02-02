#!/usr/bin/env bash
#
# Copyright 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e
SCRIPT_BASEDIR=$(dirname "$0")
ROOTDIR=$SCRIPT_BASEDIR/../..
echo ROOTDIR: $ROOTDIR
INTEGRATION_KUBECONFIG=$(readlink -f $ROOTDIR/.kubeconfig-kind-integration)

usage()
{
    cat <<EOM
Usage:
Runs integration tests for external-dns-management with mock provider
and local Kubernetes cluster in Docker (kind) or with local kube-apiserver and etcd

./run.sh [-r|--reuse] [-l] [-v] [-k|--keep] [-- <options> <for> <ginkgo>]

Options:
    -r | --reuse     reuse existing kind cluster
    -k | --keep      keep kind cluster after run for reuse or inspection
    -l               use local kube-apiserver and etcd (i.e. no kind cluster)
    -v               verbose output of script (not test itself)

For options of ginkgo run:
    ginkgo -h

Example: ./run.sh -r -k -- -v -focus=Secret -dryRun
EOM
}

while [ "$1" != "" ]; do
    case $1 in
        -r | --restart )   shift
                           NOBOOTSTRAP=true
                           ;;
        -v )               shift
                           VERBOSE=true
                           ;;
        -l )               shift
                           LOCAL_APISERVER=true
                           ;;
        -k | --keep )      shift
                           KEEP_CLUSTER=true
                           ;;
        -- )               shift
                           break
                           ;;
        * )                usage
                           exit 1
    esac
done

if [ "$LOCAL_APISERVER" == "" ]; then
  docker version > /dev/null || (echo "Local Docker installation needed" && exit 1)
fi

if [ "$VERBOSE" != "" ]; then
  set -x
fi

if [ "$NOBOOTSTRAP" == "" ] && [ "$LOCAL_APISERVER" == "" ]; then
  echo Starting Kubernetes IN Docker...

  # prepare Kubernetes IN Docker - local clusters for testing Kubernetes
  go install sigs.k8s.io/kind

  rm $INTEGRATION_KUBECONFIG || true
  touch $INTEGRATION_KUBECONFIG
  export KUBECONFIG=$INTEGRATION_KUBECONFIG

  # delete old cluster
  kind delete cluster --name integration || true

  # create K8n cluster in docker
  kind create cluster --name integration
fi

cd $ROOTDIR/test/integration

if [ "$LOCAL_APISERVER" != "" ]; then
  unset USE_EXISTING_CLUSTER
  echo using controller runtime envtest

  K8S_VERSION=1.24.2
  KUBEBUILDER_DIR=$(realpath -m kubebuilder_${K8S_VERSION})
  if [ ! -d "$KUBEBUILDER_DIR" ]; then
    curl -sSL "https://go.kubebuilder.io/test-tools/${K8S_VERSION}/$(go env GOOS)/$(go env GOARCH)" | tar -xvz
    mv kubebuilder "$KUBEBUILDER_DIR"
  fi
  export KUBEBUILDER_ASSETS="${KUBEBUILDER_DIR}/bin"
else
  export USE_EXISTING_CLUSTER=true
  export KUBECONFIG=$INTEGRATION_KUBECONFIG
  kubectl cluster-info
fi

# run test suite
GINKGO=${GINKGO:-ginkgo}
${GINKGO} -fail-fast -trace "$@"
RETCODE=$?

cd -

# cleanup
if [ "$KEEP_CLUSTER" == "" ] && [ "$LOCAL_APISERVER" == "" ]; then
  unset KUBECONFIG
  kind delete cluster --name integration
fi

exit $RETCODE
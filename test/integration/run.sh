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

set -ex

SCRIPT_BASEDIR=$(dirname "$0")
#ROOTDIR=$SCRIPT_BASEDIR/../..
ROOTDIR=$GOPATH/src/github.com/gardener/external-dns-management

usage()
{
    cat <<EOM
Usage:
Runs integration tests for external-dns-management with mock provider
and local Kubernetes cluster in Docker (kind)

./run.sh [-r|--reuse] [-v] [-k|--keep] [-- <options> <for> <ginkgo>]

Options:
    -r | --reuse     reuse existing kind cluster
    -k | --keep      keep kind cluster after run for reuse or inspection
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

if [ "$EXTERNAL" == "" ]; then
  docker version > /dev/null || (echo "Local Docker installation needed" && exit 1)
fi

if [ "$VERBOSE" != "" ]; then
  set -x
fi

if [ "$NOBOOTSTRAP" == "" ] && [ "$EXTERNAL" == "" ]; then
  echo Starting Kubernetes IN Docker...

  # prepare Kubernetes IN Docker - local clusters for testing Kubernetes
  go get sigs.k8s.io/kind

  # delete old cluster
  kind delete cluster --name integration || true

  # create K8n cluster in docker
  kind create cluster --name integration 
fi

if [ "$EXTERNAL" != "" ]; then
  echo using GKE cluster

  gcloud container clusters get-credentials $GKE_CLUSTER --project $GKE_PROJECT --zone $GKE_ZONE

  kubectl config view --minify=true --raw > /tmp/kubeconfig-gke.yaml
  # set KUBECONFIG
  export KUBECONFIG=/tmp/kubeconfig-gke.yaml
else
  # set KUBECONFIG
  export KUBECONFIG=$(kind get kubeconfig-path --name="integration")
fi

kubectl cluster-info

# install ginkgo if missing
which ginkgo || go install github.com/onsi/ginkgo/ginkgo

# run test suite
cd $ROOTDIR/test/integration && ginkgo -failFast "$@" ; cd -

# cleanup
if [ "$KEEP_CLUSTER" == "" ] && [ "$EXTERNAL" == "" ]; then
  unset KUBECONFIG
  kind delete cluster --name integration
fi

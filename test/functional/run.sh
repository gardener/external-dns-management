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

usage()
{
    cat <<EOM
Usage:
Runs functional tests for external-dns-management for all provider using secrets from a
functest-config.yaml file (see functest-config-template.yaml for details how it should look).

./run.sh [--no-dns] [-f <functest-config.yaml>] [-r|--reuse] [-l] [-v] [-k|--keep] [-- <options> <for> <ginkgo>]

Options:
    -r | --reuse        reuse existing kind cluster
    -k | --keep         keep kind cluster after run for reuse or inspection
    -l                  use local kube-apiserver and etcd (i.e. no kind cluster)
    -v                  verbose output of script (not test itself)
    --no-dns            do not perform DNS lookups (for faster testing)
    -f <config.yaml>    path to functest configuration file (defaults to ../../local/functest-config.yaml)

For options of ginkgo run:
    ginkgo -h

Example: ./run.sh -r -k -- -v -focus=aws -dryRun
EOM
}

FUNCTEST_CONFIG=$ROOTDIR/local/functest-config.yaml
DNS_LOOKUP=true

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
        -f )               shift
                           FUNCTEST_CONFIG=$1
                           shift
                           ;;
        --no-dns )         shift
                           DNS_LOOKUP=false
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
  go get sigs.k8s.io/kind

  # delete old cluster
  kind delete cluster --name integration || true

  # create K8n cluster in docker
  kind create cluster --name integration
fi


if [ "$LOCAL_APISERVER" != "" ]; then
  echo using local kube-apiserver and etcd

  # download kube-apiserver, etcd, and kubectl executables from kubebuilder release
  KUBEBUILDER_VERSION=1.0.8
  ARCH=$(go env GOARCH)
  GOOS=$(go env GOOS)
  KUBEBUILDER_BIN_DIR=$(realpath -m kubebuilder_${KUBEBUILDER_VERSION}_${GOOS}_${ARCH}/bin)
  if [ ! -d $KUBEBUILDER_BIN_DIR ]; then
    curl -Ls https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_${GOOS}_${ARCH}.tar.gz | tar xz
  fi
  export PATH=$KUBEBUILDER_BIN_DIR:$PATH
  mkdir -p $KUBEBUILDER_BIN_DIR/../var

  # starting etcd
  echo Starting Etcd
  rm -rf default.etcd
  if [ "$VERBOSE" != "" ]; then
    $KUBEBUILDER_BIN_DIR/etcd &
  else
    $KUBEBUILDER_BIN_DIR/etcd >/dev/null 2>&1 &
  fi
  PID_ETCD=$!
  trap "kill $PID_ETCD" SIGINT SIGTERM EXIT

  # starting kube-apiserver
  echo Starting Kube API Server
  if [ "$VERBOSE" != "" ]; then
    $KUBEBUILDER_BIN_DIR/kube-apiserver --etcd-servers http://localhost:2379 --cert-dir $KUBEBUILDER_BIN_DIR/../var &
  else
    $KUBEBUILDER_BIN_DIR/kube-apiserver --etcd-servers http://localhost:2379 --cert-dir $KUBEBUILDER_BIN_DIR/../var >/dev/null 2>&1 &
  fi
  PID_APISERVER=$!
  trap "kill $PID_APISERVER && kill $PID_ETCD" SIGINT SIGTERM EXIT
  sleep 3

  # create local kubeconfig
  cat > /tmp/kubeconfig-local.yaml << EOF
apiVersion: v1
clusters:
- cluster:
    server: http://localhost:8080
  name: local
contexts:
- context:
    cluster: local
  name: local-ctx
current-context: local-ctx
kind: Config
preferences: {}
users: []
EOF
  export KUBECONFIG=/tmp/kubeconfig-local.yaml
else
  export KUBECONFIG=$(kind get kubeconfig-path --name="integration")
fi

kubectl cluster-info

# install ginkgo if missing
which ginkgo || go install github.com/onsi/ginkgo/ginkgo

FUNCTEST_CONFIG=$FUNCTEST_CONFIG DNS_LOOKUP=$DNS_LOOKUP ginkgo -p "$@"

RETCODE=$?

cd -

# cleanup
if [ "$KEEP_CLUSTER" == "" ] && [ "$LOCAL_APISERVER" == "" ]; then
  unset KUBECONFIG
  kind delete cluster --name integration
fi

exit $RETCODE
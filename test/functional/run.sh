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
ROOTDIR=$(readlink -f $SCRIPT_BASEDIR/../..)
echo ROOTDIR: $ROOTDIR
INTEGRATION_KUBECONFIG=$ROOTDIR/.kubeconfig-kind-integration

cd $SCRIPT_BASEDIR

FUNCTEST_CONFIG=$ROOTDIR/local/functest-config.yaml
DNS_LOOKUP=true
DNS_COMPOUND=false
DNS_SERVER=8.8.4.4
RUN_CONTROLLER=true
GLOBAL_LOCK_URL=https://kvdb.io/8Kr6JtkwHUrq96Wk5aogEK/functest-lock

usage()
{
    cat <<EOM
Usage:
Runs functional tests for external-dns-management for all provider using secrets from a
functest-config.yaml file (see functest-config-template.yaml for details how it should look).

./run.sh [--no-dns] [-f <functest-config.yaml>] [-r|--reuse] [-l] [-v] [-k|--keep] [--dns-server <dns-server>] [--no-controller] [--compound] [-- <options> <for> <ginkgo>]

Options:
    -r | --reuse           reuse existing kind cluster
    -k | --keep            keep kind cluster after run for reuse or inspection
    -l                     use local kube-apiserver and etcd (i.e. no kind cluster)
    -v                     verbose output of script (not test itself)
    --dns-server <server>  dns server to use for DNS lookups (defaults to $DNS_SERVER)
    --no-dns               do not perform DNS lookups (for faster testing)
    -f <config.yaml>       path to functest configuration file (defaults to $FUNCTEST_CONFIG)
    --compound             use compound controller
    --no-controller        do not start the dns-controller-manager

For options of ginkgo run:
    ginkgo -h

Example: ./run.sh -r -k --no-dns -- -v -focus=aws -dryRun
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
        -f )               shift
                           FUNCTEST_CONFIG=$1
                           shift
                           ;;
        --no-dns )         shift
                           DNS_LOOKUP=false
                           ;;
        --dns-server )     shift
                           DNS_LOOKUP=$1
                           shift
                           ;;
        --compound )       shift
                           DNS_COMPOUND=true
                           ;;
        --no-controller )  shift
                           RUN_CONTROLLER=false
                           ;;
        -- )               shift
                           break
                           ;;
        * )                echo unknown arg $1
                           usage
                           exit 1
    esac
done

trapHandler()
{
  if [[ -n "$LOCK_VALUE" ]]; then
    curl -s -X DELETE $GLOBAL_LOCK_URL
    echo --- Unlocked global functest lock ---
  fi

  if [[ -n "$PID_APISERVER" ]]; then
    kill $PID_APISERVER
  fi

  if [[ -n "$PID_ETCD" ]]; then
    kill $PID_ETCD
  fi

  if [[ -n "$PID_CONTROLLER" ]]; then
    kill $PID_CONTROLLER
  fi
}

globalLock()
{
    if [[ -z "$GLOBAL_LOCK_URL" ]]; then
      return
    fi

    if [[ -n "$FORCE_UNLOCK" ]]; then
      curl -s -X DELETE $GLOBAL_LOCK_URL
    fi

    echo Waiting for global functest lock...
    i="600" # wait for maximal 600 seconds

    while [ $i -gt 0 ]; do
      val=$(curl -s $GLOBAL_LOCK_URL)
      if [ "$val" = "Not Found" ]; then
        break
      fi
      sleep 1
      i=$[$i-1]
      if ! ((i % 15)); then
        echo "Still waiting for global functest lock... (for at most $i seconds )"
      fi
    done

    if [ "$val" != "Not Found" ]; then
      echo "Cannot retrieve global functest lock: LOCK_VALUE=$val"
      exit 1
    fi

    LOCK_VALUE="$(date +%s | sha256sum | base64 | head -c 32)@$(hostname)"

    curl -s -d $LOCK_VALUE $GLOBAL_LOCK_URL

    echo '--- Locked global functest lock ('$LOCK_VALUE') ---'
}

trap trapHandler SIGINT SIGTERM EXIT

if [ "$LOCAL_APISERVER" == "" ]; then
  docker version > /dev/null || (echo "Local Docker installation needed" && exit 1)
fi

if [ "$VERBOSE" != "" ]; then
  set -x
fi

globalLock

if [ "$NOBOOTSTRAP" == "" ] && [ "$LOCAL_APISERVER" == "" ]; then
  echo Starting Kubernetes IN Docker...

  # prepare Kubernetes IN Docker - local clusters for testing Kubernetes
  go install -mod=vendor sigs.k8s.io/kind


  rm $INTEGRATION_KUBECONFIG || true
  touch $INTEGRATION_KUBECONFIG
  export KUBECONFIG=$INTEGRATION_KUBECONFIG

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

  # starting kube-apiserver
  echo Starting Kube API Server
  if [ "$VERBOSE" != "" ]; then
    $KUBEBUILDER_BIN_DIR/kube-apiserver --etcd-servers http://localhost:2379 --cert-dir $KUBEBUILDER_BIN_DIR/../var &
  else
    $KUBEBUILDER_BIN_DIR/kube-apiserver --etcd-servers http://localhost:2379 --cert-dir $KUBEBUILDER_BIN_DIR/../var >/dev/null 2>&1 &
  fi
  PID_APISERVER=$!
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
  export KUBECONFIG=$INTEGRATION_KUBECONFIG
fi

kubectl cluster-info

if [ "$RUN_CONTROLLER" == "true" ]; then
  if [ "$DNS_COMPOUND" == "true" ]; then
    go build -mod=vendor -race -o $ROOTDIR/dns-controller-manager-compound $ROOTDIR/cmd/compound
    $ROOTDIR/dns-controller-manager-compound --controllers=dnscontrollers,infoblox-dns --identifier=functest --omit-lease >/tmp/dnsmgr-functional.log 2>&1 &
    PID_CONTROLLER=$!
  else
    go build -mod=vendor -race -o $ROOTDIR/dns-controller-manager $ROOTDIR/cmd/dns
    $ROOTDIR/dns-controller-manager --controllers=dnscontrollers,infoblox-dns --identifier=functest --omit-lease > /tmp/dnsmgr-functional.log 2>&1 &
    PID_CONTROLLER=$!
  fi
else
  if [ "$DNS_COMPOUND" == "true" ]; then
    echo dns-controller-manager-compound must be started with arguments: '--controllers=dnscontrollers --identifier=functest'
  else
    echo dns-controller-manager must be started with arguments: '--controllers=dnscontrollers --identifier=functest'
  fi
fi

# install ginkgo
go install -mod=vendor github.com/onsi/ginkgo/ginkgo

GOFLAGS="-mod=vendor" FUNCTEST_CONFIG=$FUNCTEST_CONFIG DNS_LOOKUP=$DNS_LOOKUP DNS_SERVER=$DNS_SERVER DNS_COMPOUND=$DNS_COMPOUND ginkgo -p "$@"

RETCODE=$?

cd -

# cleanup
if [ "$KEEP_CLUSTER" == "" ] && [ "$LOCAL_APISERVER" == "" ]; then
  unset KUBECONFIG
  kind delete cluster --name integration
fi

exit $RETCODE
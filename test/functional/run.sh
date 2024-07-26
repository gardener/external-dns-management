#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -e
SCRIPT_BASEDIR=$(dirname "$0")
ROOTDIR=$(readlink -f $SCRIPT_BASEDIR/../..)
echo ROOTDIR: $ROOTDIR
INTEGRATION_KUBECONFIG=$ROOTDIR/.kubeconfig-kind-integration

cd $SCRIPT_BASEDIR

FUNCTEST_CONFIG=$ROOTDIR/local/functest-config.yaml
DNS_LOOKUP=true
DNS_SERVER=8.8.4.4
RUN_CONTROLLER=true
GLOBAL_LOCK_URL=https://kvdb.io/8Kr6JtkwHUrq96Wk5aogEK/functest-lock

usage()
{
    cat <<EOM
Usage:
Runs functional tests for external-dns-management for all provider using secrets from a
functest-config.yaml file (see functest-config-template.yaml for details how it should look).

./run.sh [--no-dns] [-f <functest-config.yaml>] [-r|--reuse] [-v] [-k|--keep] [--dns-server <dns-server>] [--no-controller] [-- <options> <for> <ginkgo>]

Options:
    -r | --reuse           reuse existing kind cluster
    -k | --keep            keep kind cluster after run for reuse or inspection
    -v                     verbose output of script (not test itself)
    --dns-server <server>  dns server to use for DNS lookups (defaults to $DNS_SERVER)
    --no-dns               do not perform DNS lookups (for faster testing)
    -f <config.yaml>       path to functest configuration file (defaults to $FUNCTEST_CONFIG)
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
  go install sigs.k8s.io/kind


  rm $INTEGRATION_KUBECONFIG || true
  touch $INTEGRATION_KUBECONFIG
  export KUBECONFIG=$INTEGRATION_KUBECONFIG

  # delete old cluster
  kind delete cluster --name integration || true

  # create K8n cluster in docker
  kind create cluster --name integration
fi


if [ "$LOCAL_APISERVER" != "" ]; then
  echo not supported
  exit 1
else
  export KUBECONFIG=$INTEGRATION_KUBECONFIG
fi

kubectl cluster-info

if [ "$RUN_CONTROLLER" == "true" ]; then
  go build -race -o $ROOTDIR/dns-controller-manager $ROOTDIR/cmd/compound
  $ROOTDIR/dns-controller-manager --controllers=dnscontrollers --identifier=functest --omit-lease > /tmp/dnsmgr-functional.log 2>&1 &
  PID_CONTROLLER=$!
else
  echo dns-controller-manager must be started with arguments: '--controllers=dnscontrollers --identifier=functest'
fi

GINKGO=${GINKGO:-ginkgo}
FUNCTEST_CONFIG=$FUNCTEST_CONFIG DNS_LOOKUP=$DNS_LOOKUP DNS_SERVER=$DNS_SERVER ${GINKGO} -v -p "$@"

RETCODE=$?

cd -

# cleanup
if [ "$KEEP_CLUSTER" == "" ] && [ "$LOCAL_APISERVER" == "" ]; then
  unset KUBECONFIG
  kind delete cluster --name integration
fi

exit $RETCODE
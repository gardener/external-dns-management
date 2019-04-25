#!/bin/bash

set -e

SCRIPT_BASEDIR=$(dirname "$0")
ROOTDIR=$(realpath "$SCRIPT_BASEDIR/../..")

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

docker version > /dev/null || (echo "Local Docker installation needed" && exit 1)

if [ "$VERBOSE" != "" ]; then
  set -x
fi

if [ "$NOBOOTSTRAP" == "" ]; then
  # prepare Kubernetes IN Docker - local clusters for testing Kubernetes
  go get sigs.k8s.io/kind

  # delete old cluster
  kind delete cluster --name integration || true

  # create K8n cluster in docker
  kind create cluster --name integration 
fi

# store tmp kubeconfig
export KUBECONFIG=$(kind get kubeconfig-path --name="integration")
kubectl cluster-info

# install ginkgo if missing
which ginkgo || go install github.com/onsi/ginkgo/ginkgo

# run test suite
cd $ROOTDIR/test/integration && ginkgo -failFast "$@" ; cd -

# cleanup
if [ "$KEEP_CLUSTER" == "" ]; then
  unset KUBECONFIG
  kind delete cluster --name integration
fi


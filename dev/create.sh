#!/bin/sh 
set -ex

for command in minikube kubectl; do
    if ! command -v "${command}" > /dev/null; then
        2>&1 echo "error: ${command} not installed"
        exit 1
    fi
done

if [ ! -d "/workspaces/external-dns-routeros-provider" ]; then
  2>&1 echo "error: must be run from devcontainer"
  exit 1
fi

echo "delete minikube cluster if exists"
minikube delete || true

echo "create minikube cluster"
minikube start --force

echo "remove routeros container if exists"
(docker stop routeros && docker rm routeros) || true

echo "start routeros container"
kvm_arg=""
cpu="qemu64"
if [ -e "/dev/kvm" ]; then
  kvm_arg="--device=/dev/kvm"
  cpu="host"
fi
docker run --name=routeros --rm --detach --publish=80:80 --publish=8728:8728 --cap-add=NET_ADMIN --device=/dev/net/tun ${kvm_arg} --platform=linux/amd64 evilfreelancer/docker-routeros -cpu ${cpu}

echo "apply external-dns crds"
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/external-dns/master/docs/contributing/crd-source/crd-manifest.yaml

echo "create dns records"
kubectl apply -f ./dev/records.yaml
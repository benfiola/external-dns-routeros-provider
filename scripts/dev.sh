#!/bin/sh 
set -e

for command in minikube kubectl; do
    if ! command -v "${command}" > /dev/null; then
        2>&1 echo "error: ${command} not installed"
        exit 1
    fi
done

if [ ! -d "/workspaces/external-dns-mikrotik-webhook" ]; then
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
cpu="host"
if [ ! -f "/dev/kvm" ]; then
  cpu="qemu64"
fi
docker run --name routeros --rm --detach --publish 8728:8728 --cap-add NET_ADMIN --device /dev/net/tun --platform linux/amd64 evilfreelancer/docker-routeros -cpu "${cpu}"


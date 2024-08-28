#!/bin/sh
set -ex

case $(uname -m) in
    x86_64)  arch="amd64" ;;
    arm64)   arch="arm64" ;;
    aarch64) arch="arm64" ;;
esac

# install minikube
curl -o /usr/local/bin/minikube -fsSL "https://storage.googleapis.com/minikube/releases/v1.33.1/minikube-linux-${arch}"
chmod +x /usr/local/bin/minikube

# install kubectl
curl -o /usr/local/bin/kubectl -fsSL "https://dl.k8s.io/release/v1.29.4/bin/linux/${arch}/kubectl"
chmod +x /usr/local/bin/kubectl

# install k9s
curl -fsSL -o k9s.tar.gz "https://github.com/derailed/k9s/releases/download/v0.32.4/k9s_Linux_${arch}.tar.gz"
tar xvzf k9s.tar.gz -C /usr/local/bin
rm -rf k9s.tar.gz

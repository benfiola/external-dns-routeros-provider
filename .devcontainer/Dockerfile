FROM ubuntu:jammy

ARG TARGETARCH

ENV DEBIAN_FRONTEND="noninteractive"

# install dependencies
RUN apt -y update && \
    apt -y install curl git

# install minikube
RUN mkdir -p /opt/minikube && \
    cd /opt/minikube && \
    curl -o /opt/minikube/minikube -fsSL "https://storage.googleapis.com/minikube/releases/v1.33.1/minikube-linux-${TARGETARCH}" && \
    chmod +x /opt/minikube/minikube
ENV PATH="/opt/minikube:${PATH}"

# install kubectl
RUN mkdir -p /opt/kubectl && \
    cd /opt/kubectl && \
    curl -o /opt/kubectl/kubectl -fsSL "https://dl.k8s.io/release/v1.29.4/bin/linux/${TARGETARCH}/kubectl" && \
    chmod +x /opt/kubectl/kubectl
ENV PATH="/opt/kubectl:${PATH}"

# install k9s
RUN mkdir -p /opt/k9s && \
    cd /opt/k9s && \
    curl -fsSL -o k9s.tar.gz "https://github.com/derailed/k9s/releases/download/v0.32.4/k9s_Linux_${TARGETARCH}.tar.gz" && \
    tar xvzf k9s.tar.gz && \
    rm -rf k9s.tar.gz
ENV PATH="/opt/k9s:${PATH}"
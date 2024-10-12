ASSETS ?= $(shell pwd)/.dev
DEV ?= $(shell pwd)/dev
KUBERNETES_VERSION ?= 1.30.4
MINIKUBE_VERSION ?= 1.34.0
ROUTEROS_VERSION ?= 7.16

OS = $(shell go env GOOS)
ARCH = $(shell go env GOARCH)

DOCKER_CMD = docker
EXTERNAL_DNS_MANIFEST = $(ASSETS)/external-dns.yaml
EXTERNAL_DNS_MANIFEST_SRC = $(DEV)/manifests/external-dns
KUBECONFIG = $(ASSETS)/kube-config.yaml
KUBECTL = $(ASSETS)/kubectl
KUBECTL_CMD = env KUBECONFIG=$(KUBECONFIG) $(KUBECTL)
KUBECTL_URL = https://dl.k8s.io/release/v$(KUBERNETES_VERSION)/bin/$(OS)/$(ARCH)/kubectl
KUSTOMIZE_CMD = $(KUBECTL_CMD) kustomize
MINIKUBE = $(ASSETS)/minikube
MINIKUBE_CMD = env KUBECONFIG=$(KUBECONFIG) MINIKUBE_HOME=$(ASSETS)/.minikube $(MINIKUBE)
MINIKUBE_URL = https://github.com/kubernetes/minikube/releases/download/v$(MINIKUBE_VERSION)/minikube-$(OS)-$(ARCH)

.PHONY: default
default: 

.PHONY: clean
clean: delete-minikube-cluster
	# delete asset directory
	rm -rf $(ASSETS)

.PHONY: dev-env
dev-env: create-minikube-cluster apply-manifests start-routeros wait-for-ready

.PHONY: e2e-test
e2e-test:
	go test -count=1 -v ./internal/e2e

.PHONY: create-minikube-cluster
create-minikube-cluster: $(MINIKUBE)
	# create minikube cluster
	$(MINIKUBE_CMD) start --force --kubernetes-version=$(KUBERNETES_VERSION)

.PHONY: delete-minikube-cluster
delete-minikube-cluster: $(MINIKUBE)
	# delete minikube cluster
	$(MINIKUBE_CMD) delete || true

.PHONY: start-routeros
start-routeros:
	# stop existing routeros container
	$(DOCKER_CMD) stop routeros || true
	# start routeros container
	$(DOCKER_CMD) run --name=routeros --rm --detach --publish=80:80 --publish=8728:8728 --cap-add=NET_ADMIN --device=/dev/net/tun --platform=linux/amd64 evilfreelancer/docker-routeros:$(ROUTEROS_VERSION) -cpu qemu64	

.PHONY: wait-for-ready
wait-for-ready:
	# wait for routeros to be connectable
	while true; do curl --max-time 2 -I http://localhost:80 && break; sleep 1; done;

$(ASSETS):
	# create .dev directory
	mkdir -p $(ASSETS)

.PHONY: install-tools
install-tools: $(KUBECTL) $(MINIKUBE)

$(KUBECTL): | $(ASSETS)
	# install kubectl
	# download
	curl -o $(KUBECTL) -fsSL $(KUBECTL_URL)
	# make kubectl executable
	chmod +x $(KUBECTL)

$(MINIKUBE): | $(ASSETS)
	# install minikube
	# download
	curl -o $(MINIKUBE) -fsSL $(MINIKUBE_URL)
	# make executable
	chmod +x $(MINIKUBE)

.PHONY: apply-manifests
apply-manifests: $(EXTERNAL_DNS_MANIFEST) $(KUBECTL) 
	# apply external-dns manifest
	$(KUBECTL_CMD) apply -f $(EXTERNAL_DNS_MANIFEST)

.PHONY: generate-manifests
generate-manifests: $(EXTERNAL_DNS_MANIFEST)

$(EXTERNAL_DNS_MANIFEST): $(KUBECTL) | $(ASSETS)
	# generate external dns manifests
	$(KUSTOMIZE_CMD) $(EXTERNAL_DNS_MANIFEST_SRC) > $(EXTERNAL_DNS_MANIFEST)
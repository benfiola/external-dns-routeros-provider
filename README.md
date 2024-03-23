# external-dns-mikrotik-webhook

This repo contains the code required to run a [webhook provider](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/webhook-provider.md) capable of working with [external-dns](https://github.com/kubernetes-sigs/external-dns/tree/master) to manage kubernetes
DNS records on Mikrotik routers running RouterOS.  It communicates over socket via RouterOS's [APIs]().

---

## Usage

The docker image for this project is located at 
This webhook server is intended to be run as a sidecar alongside `external-dns` - such that the webhook is connectable via `localhost:8888`.
## Configuration


# external-dns-routeros-provider

<p align="center">
    <em>A RouterOS provider for external-dns</em>
</p>

This repo provides an external-dns [webhook provider](https://kubernetes-sigs.github.io/external-dns/v0.14.2/tutorials/webhook-provider/) capable of integrating with devices running Mikrotik's [RouterOS](https://help.mikrotik.com/docs/display/ROS/API).

---

## Usage

The docker image can be pulled from [docker.io/benfiola/external-dns-routeros-provider:latest](https://hub.docker.com/r/benfiola/external-dns-routeros-provider).

This webhook server is intended to be run as a sidecar alongside `external-dns` - such that the webhook is connectable via `localhost:8888`. An example deployment can be found [here](./manifests/example-deployment.yaml).

## Configuration

Configuring the webhook can be done via the environment or via CLI arguments.

| CLI                    | Environment Variable                                | Description                                                                            |
| ---------------------- | --------------------------------------------------- | -------------------------------------------------------------------------------------- |
| --filter-exclude       | EXTERNAL_DNS_ROUTEROS_PROVIDER_FILTER_EXCLUDE       | (Optional) domain name to exclude from webhook processing - can be used multiple times |
| --filter-include       | EXTERNAL_DNS_ROUTEROS_PROVIDER_FILTER_INCLUDE       | (Optional) domain name to include in webhook processing - can be used multiple times   |
| --filter-regex-exclude | EXTERNAL_DNS_ROUTEROS_PROVIDER_FILTER_REGEX_EXCLUDE | (Optional) domain name regex to exclude from webhook processing                        |
| --filter-regex-include | EXTERNAL_DNS_ROUTEROS_PROVIDER_FILTER_REGEX_INCLUDE | (Optional) domain name regex to include in webhook processing                          |
| --log-level            | EXTERNAL_DNS_ROUTEROS_PROVIDER_LOG_LEVEL            | (Optional) log level (`error, warning, info, debug`), default: `info`                  |
| --routeros-address     | EXTERNAL_DNS_ROUTEROS_PROVIDER_ROUTEROS_ADDRESS     | routeros device `<host>:<port>`                                                        |
| --routeros-password    | EXTERNAL_DNS_ROUTEROS_PROVIDER_ROUTEROS_PASSWORD    | routeros password                                                                      |
| --routeros-username    | EXTERNAL_DNS_ROUTEROS_PROVIDER_ROUTEROS_USERNAME    | routeros username                                                                      |
| --server-host          | EXTERNAL_DNS_ROUTEROS_PROVIDER_SERVER_HOST          | (Optional) server host to listen on, default: `127.0.0.1`                              |
| --server-port          | EXTERNAL_DNS_ROUTEROS_PROVIDER_SERVER_PORT          | (Optional) server port to listen on, default: `8888`                                   |

## Development

I personally use [vscode](https://code.visualstudio.com/) as an IDE. For a consistent development experience, this project is also configured to utilize [devcontainers](https://containers.dev/). If you're using both - and you have the [Dev Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers) installed - you can follow the [introductory docs](https://code.visualstudio.com/docs/devcontainers/tutorial) to quickly get started.

NOTE: Helper scripts are written under the assumption that they're being executed within a dev container.

### Installing tools

From the project root, run the following to install useful tools. Currently, this includes:

- kubectl
- minikube

```shell
cd /workspaces/external-dns-routeros-provider
make install-tools
```

### Creating a development environment

From the project root, run the following to create a development environment to test the provider with:

```shell
cd /workspaces/external-dns-routeros-provider
make dev-env
```

This will:

- Create a new minikube cluster
- Apply external-dns CRDs
- Start a local routeros container
- Wait for routeros to be accessible

### Run end-to-end tests

With a development environment deployed, you can run end-to-end tests to confirm the provider functions as expected:

```shell
cd /workspaces/external-dns-routeros-provider
make e2e-test
```

### Creating a debug script

Copy the [./dev/dev.go.template](./dev/dev.go.template) script to `./dev/dev.go`, then run it to start the provider. `./dev/dev.go` is ignored by git and can be modified as needed to help facilitate local development.

Additionally, the devcontainer is configured with a vscode launch configuration that points to `./dev/dev.go`. You should be able to launch (and attach a debugger to) the webhook via this vscode launch configuration.

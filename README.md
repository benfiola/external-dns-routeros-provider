# external-dns-mikrotik-webhook

This repo contains the code required to run a [webhook provider](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/webhook-provider.md) capable of working with [external-dns](https://github.com/kubernetes-sigs/external-dns/tree/master) to manage kubernetes
DNS records on Mikrotik routers running RouterOS. It communicates over socket via RouterOS's [APIs](https://help.mikrotik.com/docs/display/ROS/API).

---

## Usage

The docker image can be pulled from [docker.io/benfiola/external-dns-mikrotik-webhook:latest](https://hub.docker.com/r/benfiola/external-dns-mikrotik-webhook).

This webhook server is intended to be run as a sidecar alongside `external-dns` - such that the webhook is connectable via `localhost:8888`.

## Configuration

Configuring the webhook can be done via the environment or via CLI arguments.

| CLI                      | Env                                 | Description                                                                            |
| ------------------------ | ----------------------------------- | -------------------------------------------------------------------------------------- |
| --domain-filter          | EXTERNAL_DNS_DOMAIN_FILTER          | (Optional) domain name to include in webhook processing - can be used multiple times   |
| --exclude-domains        | EXTERNAL_DNS_EXCLUDE_DOMAINS        | (Optional) domain name to exclude from webhook processing - can be used multiple times |
| --regex-domain-filter    | EXTERNAL_DNS_REGEX_DOMAIN_FILTER    | (Optional) domain name regex to include in webhook processing                          |
| --regex-domain-exclusion | EXTERNAL_DNS_REGEX_DOMAIN_EXCLUSION | (Optional) domain name regex to exclude from webhook processing                        |
| --log-level              | EXTERNAL_DNS_LOG_LEVEL              | (Optional) log level (`error, warning, info, debug`), default: `info`                  |
| --routeros-host          | ROUTEROS_HOST                       | Hostname/IP address of routeros host                                                   |
| --routeros-password      | ROUTEROS_PASSWORD                   | Password of user on routeros host                                                      |
| --routeros-username      | ROUTEROS_USERNAME                   | Username of user on routeros host                                                      |

## Development

I personally use [vscode](https://code.visualstudio.com/) as an IDE. For a consistent development experience, this project is also configured to utilize [devcontainers](https://containers.dev/). If you're using both - and you have the [Dev Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers) installed - you can follow the [introductory docs](https://code.visualstudio.com/docs/devcontainers/tutorial) to quickly get started.

NOTE: Helper scripts are written under the assumption that they're being executed within a dev container.

### Creating a cluster

From the project root, run the following to create a development cluster to test the webhook with:

```shell
cd /workspaces/external-dns-mikrotik-webhook
./scripts/dev.sh
```

This will:

- Delete an existing dev cluster if one exists
- Create a new dev cluster
- Delete an existing routeros container if one exists
- Create a new routeros container

### Creating a launch script

Copy the [dev.template.py](./dev.template.py) script to `dev.py`, then run it to start a sample webhook.

If placed in the top-level directory, `dev.py` is gitignored and you can change this file as needed without worrying about committing it to git.

Additionally, the devcontainer is configured with vscode launch configurations that point to a top-level `dev.py` file. You should be able to launch (and attach a debugger to) the webhook by launching it natively through vscode.

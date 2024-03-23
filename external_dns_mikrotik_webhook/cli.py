import asyncio
import logging
import re

import click
import uvloop

from external_dns_mikrotik_webhook.logs import configure_logging
from external_dns_mikrotik_webhook.mikrotik import Client as MikrotikClient
from external_dns_mikrotik_webhook.provider import DomainFilter, Provider
from external_dns_mikrotik_webhook.webhook import Webhook


def main():
    """
    Main entry point for the module
    """
    uvloop.install()
    grp_main()


@click.group()
def grp_main():
    """
    Main group for commands to attach to
    """
    pass


def regex_type(val: str) -> re.Pattern:
    """
    Helper to parse regex patterns from strings via the `click` library.
    """
    try:
        return re.compile(val)
    except Exception as e:
        raise ValueError(f"not regex") from e


def log_level_type(val: str) -> int:
    """
    Helper that parses a human readable log level (e.g., debug, info) into
    a value recognized by `logging`.
    """
    int_value = getattr(logging, val.upper())
    if not isinstance(int_value, int):
        raise ValueError(f"not log level")
    return int_value


@grp_main.command("webhook-server")
@click.option(
    "--domain-filter",
    "_df_include",
    multiple=True,
    default=None,
    envvar="EXTERNAL_DNS_DOMAIN_FILTER",
)
@click.option(
    "--exclude-domains",
    "_df_exclude",
    multiple=True,
    default=None,
    envvar="EXTERNAL_DNS_EXCLUDE_DOMAINS",
)
@click.option(
    "--regex-domain-filter",
    "df_regex_include",
    type=regex_type,
    envvar="EXTERNAL_DNS_REGEX_DOMAIN_FILTER",
)
@click.option(
    "--regex-domain-exclusion",
    "df_regex_exclude",
    type=regex_type,
    envvar="EXTERNAL_DNS_REGEX_DOMAIN_EXCLUSION",
)
@click.option(
    "--log-level",
    type=log_level_type,
    envvar="EXTERNAL_DNS_LOG_LEVEL",
)
@click.option("--routeros-host", type=str, required=True, envvar="ROUTEROS_HOST")
@click.option(
    "--routeros-password", type=str, required=True, envvar="ROUTEROS_PASSWORD"
)
@click.option(
    "--routeros-username", type=str, required=True, envvar="ROUTEROS_USERNAME"
)
def cmd_server(
    routeros_host: str,
    routeros_password: str,
    routeros_username: str,
    _df_include: tuple[str] | None = None,
    _df_exclude: tuple[str] | None = None,
    df_regex_include: re.Pattern | None = None,
    df_regex_exclude: re.Pattern | None = None,
    log_level: int | None = None,
):
    async def inner():
        nonlocal _df_include, _df_exclude
        configure_logging(log_level=log_level)
        df_include = list(_df_include) if _df_include else None
        df_exclude = list(_df_exclude) if _df_exclude else None
        domain_filter = DomainFilter(
            include=df_include,
            exclude=df_exclude,
            regex_include=df_regex_include,
            regex_exclude=df_regex_exclude,
        )
        client = MikrotikClient(
            host=routeros_host, password=routeros_password, username=routeros_username
        )
        provider = Provider(client, domain_filter=domain_filter)
        webhook = Webhook(provider)
        await webhook.run()

    asyncio.run(inner())


if __name__ == "__main__":
    main()

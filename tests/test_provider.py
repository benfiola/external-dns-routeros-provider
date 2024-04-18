import pathlib
from typing import cast

import dotenv
import pykrotik

import external_dns_mikrotik_webhook.provider
from tests.common import integration_test


def provider_from_env():
    env_file = pathlib.Path(__file__).parent.parent.joinpath("local/.env")
    data = cast(dict[str, str], dotenv.dotenv_values(env_file))
    client = pykrotik.Client(
        host=data["ROUTEROS_HOST"],
        password=data["ROUTEROS_PASSWORD"],
        username=data["ROUTEROS_USERNAME"],
    )
    return external_dns_mikrotik_webhook.provider.Provider(client=client)


@integration_test
def test_provider_adjust_endpoints():
    provider = provider_from_env()


@integration_test
def test_provider_apply_changes():
    provider = provider_from_env()


@integration_test
def test_provider_list_records():
    provider = provider_from_env()


@integration_test
def test_provider_get_domain_filter():
    provider = provider_from_env()

import asyncio
import logging

from pykrotik import Client

from external_dns_mikrotik_webhook.logs import configure_logging
from external_dns_mikrotik_webhook.provider import Provider
from external_dns_mikrotik_webhook.webhook import Webhook


async def main():
    configure_logging(logging.DEBUG)
    webhook = Webhook(
        provider=Provider(
            client=Client(host="localhost", username="admin", password="")
        )
    )
    await webhook.run()


if __name__ == "__main__":
    asyncio.run(main())

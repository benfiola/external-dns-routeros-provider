import logging
from typing import Any, Awaitable, Callable, TypeVar, cast

import fastapi
import fastapi.exception_handlers
import pydantic
import uvicorn.config
import uvicorn.server

from external_dns_mikrotik_webhook.provider import Changes, Endpoint, Provider

logger = logging.getLogger(__name__)

Dependency = TypeVar("Dependency")


def depends(value: Callable[..., Awaitable[Dependency]]) -> Dependency:
    """
    Typed wrapper around `fastapi.Depends` to avoid needing to declare the type *and* assign a default:

    `async def view_func(dependency: Dependency = fastapi.Depends(get_dependency))`
    becomes
    `async def view_func(dependency=depends(get_dependency))`
    """
    return fastapi.Depends(cast(Any, value))


class UnsupportedMediaType(fastapi.HTTPException):
    """
    Common exception when an unsupported media type is encountered
    """

    def __init__(self, media_type: str):
        super().__init__(400, f"Unsupported header: {media_type}")


class JSONResponse(fastapi.responses.Response):
    """
    Custom JSON response object.

    These classes handle serializing pydantic models/type adapters into bytes directly,
    while additionally managing headers based upon the media-type of the response.

    Used in conjuction with `response_cls_provider` - allows view functions to more
    easily create custom responses without relying too heavily on the magic fastapi
    does when direct responses aren't returned.
    """

    media_type = "application/json"

    def __init__(
        self,
        *,
        model: pydantic.BaseModel | None = None,
        type_adapter: pydantic.TypeAdapter | None = None,
        data: Any | None = None,
    ):
        # serialize data by alias (as pydantic converts data from aliases into pythonic field names during deserialization)
        # ignore 'None' fields - external-dns omits these during deserialization
        dump_kwargs: dict = {"exclude_none": True, "by_alias": True}
        # dump a pydantic model
        if model is not None:
            data_ = model.model_dump_json(**dump_kwargs)
        # dump data according to the provided type adapter
        elif type_adapter is not None and data is not None:
            data_ = type_adapter.dump_json(data, **dump_kwargs)
        else:
            raise ValueError(f"model or (type_adapter and data) must be provided")
        super().__init__(data_)


class WebhookJSONResponse(JSONResponse):
    """
    A json response with the external-dns webhook media type
    """

    media_type = "application/external.dns.webhook+json;version=1"


# a list of response classes ( + media types) supported by the webhook
supported_responses = {JSONResponse, WebhookJSONResponse}


async def response_cls_provider(
    request: fastapi.Request,
) -> type[JSONResponse]:
    """
    Returns a response class based upon the 'accept' header of the incoming request
    """
    media_types = {resp_cls.media_type: resp_cls for resp_cls in supported_responses}

    if value := request.headers.get("accept"):
        # validate that a provided accept header is a known type
        if value not in media_types:
            raise UnsupportedMediaType(value)
        return media_types[value]

    # by default, return a typical application/json response
    return JSONResponse


def validation_exception_handler(
    request: fastapi.Request, exc: fastapi.exceptions.RequestValidationError
):
    """
    When a request fails to validate through fastapi, a 422 response is returned - but it's unclear
    *how* the request failed validation.  This is important if the request originates from external-dns
    as we need to fully support its payloads.

    This method will log validation errors to provide more data around why the request failed.
    """
    logger.exception(f"failed to validate incoming request", exc_info=exc)
    return fastapi.exception_handlers.request_validation_exception_handler(request, exc)


class Webhook(fastapi.FastAPI):
    """
    Implements the ASGI application exposing the webhooks required for external DNS.

    This class deals primarily with data transport and conversion - no implementation details should be found here.

    Reference: https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/webhook-provider.md
    """

    # the provider implements the api required to connect to routeros
    provider: Provider

    def __init__(self, provider: Provider):
        super().__init__()

        self.provider = provider

        # register endpoints
        self.get("/")(self.negotiate)
        self.post("/adjustendpoints")(self.adjust_endpoints)
        self.get("/healthz")(self.k8s_probe)
        self.get("/records")(self.list_records)
        self.post("/records")(self.apply_changes)
        self.exception_handlers[fastapi.exceptions.RequestValidationError] = (
            validation_exception_handler
        )

    async def adjust_endpoints(
        self,
        endpoints: list[Endpoint],
        response_cls=depends(response_cls_provider),
    ) -> fastapi.Response:
        """
        Calls `provider.adjust_endpoints`.
        """
        data = await self.provider.adjust_endpoints(endpoints)
        type_adapter = pydantic.TypeAdapter(list[Endpoint])
        response = response_cls(type_adapter=type_adapter, data=data)
        return response

    async def apply_changes(self, changes: Changes, response: fastapi.Response):
        """
        Calls `provider.apply_changes`.
        """
        await self.provider.apply_changes(changes)
        response.status_code = 204

    async def k8s_probe(
        self,
        response: fastapi.Response,
    ):
        """
        Health endpoint - non-200 status codes are unhealthy, 200 status codes are healthy.
        """
        response.status_code = 200

    async def negotiate(
        self, response_cls=depends(response_cls_provider)
    ) -> fastapi.Response:
        """
        Allows the client to 'negotiate' content types prior to performing operations against the provider.

        Calls `provider.get_domain_filter`.
        """
        data = await self.provider.get_domain_filter()
        response = response_cls(model=data)
        return response

    async def list_records(
        self, response_cls=depends(response_cls_provider)
    ) -> fastapi.Response:
        """
        Calls `provider.list_records`
        """
        data = await self.provider.list_records()
        type_adapter = pydantic.TypeAdapter(list[Endpoint])
        response = response_cls(type_adapter=type_adapter, data=data)
        return response

    async def run(self):
        """
        Runs the ASGI application using `uvicorn`.

        Blocks until the server is shut down.
        """
        config = Config(
            app=self,
            port=8888,
            host="0.0.0.0",
            reload=False,
            workers=1,
        )
        server = uvicorn.server.Server(config)
        await server.serve()


class Config(uvicorn.config.Config):
    def configure_logging(self) -> None:
        pass

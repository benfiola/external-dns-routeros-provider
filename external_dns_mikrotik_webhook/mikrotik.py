import asyncio
import datetime
import enum
import logging
import uuid
from typing import Annotated, Any, Literal, Union

import pydantic

logger = logging.getLogger(__name__)


class IpDnsRecordType(str, enum.Enum):
    """
    Ip record types supported by routeros
    """

    A = "A"
    AAAA = "AAAA"
    CNAME = "CNAME"
    FWD = "FWD"
    MX = "MX"
    NS = "NS"
    NXDOMAIN = "NXDOMAIN"
    SRV = "SRV"
    TXT = "TXT"


class BaseModel(pydantic.BaseModel):
    """
    Defines a base model from which other models can be defined
    """

    # config that:
    #  - allows models to be populated by alias and field name
    model_config = pydantic.ConfigDict(populate_by_name=True)


class BaseIpDnsRecord(BaseModel):
    """
    All ip records share a base set of fields - this base class is used to define all ip record modules
    """

    # flag that dictates whether an ip address record is active
    disabled: bool
    # the id for an ip address record (NOTE: prefixed with '*')
    id: str | None = pydantic.Field(default=None, alias=str(".id"))
    # rule that dictates whether a dns record matches subdomains
    match_subdomain: bool = pydantic.Field(default=False, alias=str("match-subdomain"))
    # the name of the dns record
    name: str
    # the ttl of the dns record (represented as '1w1d1h1m1s')
    ttl: str


class IpDnsARecord(BaseIpDnsRecord):
    """
    Models ip dns a record data returned from routeros
    """

    # the address the A record points to
    address: str
    type: Literal[IpDnsRecordType.A] = IpDnsRecordType.A


class IpDnsAaaaRecord(BaseIpDnsRecord):
    """
    Models ip dns a record data returned from routeros
    """

    # the address the AAAA record points to
    address: str
    type: Literal[IpDnsRecordType.AAAA] = IpDnsRecordType.AAAA


class IpDnsCnameRecord(BaseIpDnsRecord):
    """
    Models ip dns cname record data returned from routeros
    """

    # the cname for the record
    cname: str
    type: Literal[IpDnsRecordType.CNAME] = IpDnsRecordType.CNAME


class IpDnsFwdRecord(BaseIpDnsRecord):
    """
    Models ip dns fwd record data returned from routeros
    """

    # the address the FWD record points to
    forward_to: str = pydantic.Field(alias=str("forward-to"))
    type: Literal[IpDnsRecordType.FWD] = IpDnsRecordType.FWD


class IpDnsMxRecord(BaseIpDnsRecord):
    """
    Models ip dns mx record data returned from routeros
    """

    mx_preference: int = pydantic.Field(alias=str("mx-preference"))
    mx_exchange: str = pydantic.Field(alias=str("mx-exchange"))
    type: Literal[IpDnsRecordType.MX] = IpDnsRecordType.MX


class IpDnsNsRecord(BaseIpDnsRecord):
    """
    Models ip dns ns record data returned from routeros
    """

    ns: str
    type: Literal[IpDnsRecordType.NS] = IpDnsRecordType.NS


class IpDnsNxdomainRecord(BaseIpDnsRecord):
    """
    Models ip dns nxdomain record data returned from routeros
    """

    type: Literal[IpDnsRecordType.NXDOMAIN] = IpDnsRecordType.NXDOMAIN


class IpDnsSrvRecord(BaseIpDnsRecord):
    """
    Models ip dns srv record data returned from routeros
    """

    srv_port: int = pydantic.Field(alias=str("srv-port"))
    srv_priority: int = pydantic.Field(alias=str("srv-priority"))
    srv_target: str = pydantic.Field(alias=str("srv-target"))
    srv_weight: int = pydantic.Field(alias=str("srv-weight"))
    type: Literal[IpDnsRecordType.SRV] = IpDnsRecordType.SRV


class IpDnsTxtRecord(BaseIpDnsRecord):
    """
    Models ip dns txt record data returned from routeros
    """

    # the text content for the record
    text: str
    type: Literal[IpDnsRecordType.TXT] = IpDnsRecordType.TXT


# annotated discriminating union to help with parsing ip dns records
IpDnsRecord = Annotated[
    Union[
        IpDnsARecord,
        IpDnsAaaaRecord,
        IpDnsCnameRecord,
        IpDnsFwdRecord,
        IpDnsMxRecord,
        IpDnsNsRecord,
        IpDnsNxdomainRecord,
        IpDnsSrvRecord,
        IpDnsTxtRecord,
    ],
    pydantic.Field(discriminator="type"),
]


class Request:
    """
    Data container for request data
    """

    # defines the tag attached to the request (and subsequent response sentences)
    tag: str
    # defines the words used to construct this request object (without additional words added via `get_sentence()`)
    words: list[str]

    def __init__(self, words: list[str]):
        self.tag = uuid.uuid4().hex
        self.words = words

    def get_sentence(self) -> list[str]:
        """
        Assembles a sentence using the provided words, the generated tag and an empty string to indicate the end of the sentence
        """
        return [*self.words, *to_api_attribute_words({".tag": self.tag}), ""]


class ResponseSentence:
    """
    Represents a sentence read from routeros
    """

    # 'api_attributes' holds additional metadata (e.g., tags)
    api_attributes: dict[str, str]
    # 'attributes' holds the requested response data
    attributes: dict[str, str]
    # defines the type of sentence (!re, !done, !trap)
    type: str

    def __init__(self, type: str):
        self.api_attributes = {}
        self.attributes = {}
        self.type = type


class ResponseStatus(str, enum.Enum):
    """
    Represents the status of a response received for a sent request.

    An 'error' status occurs when any sentence within a response has the '!trap' type
    A 'success' status occurs when a '!done' sentence is received *without* any '!trap' type sentences

    Because a '!done' message needs to be received for a response to be considered complete - status
    alone does not communicate whether a response is finished.
    """

    InProgress = "in-progress"
    Error = "error"
    Success = "success"


class Response:
    """
    Data container for response data.
    """

    # event signaled with a '!done' sentence is received - optimizes polling for status change
    completion_event: asyncio.Event
    # the request attached to this response
    request: Request
    # the sentences received for the request
    sentences: list[ResponseSentence]
    # the status of the request
    status: ResponseStatus

    def __init__(self, request: Request):
        self.completion_event = asyncio.Event()
        self.request = request
        self.sentences = []
        self.status = ResponseStatus.InProgress

    @property
    def tag(self) -> str:
        """
        Returns the request's tag
        """
        return self.request.tag

    def is_complete(self) -> bool:
        """
        Helper method indicating whether the response has been fully received from routeros
        """
        return self.completion_event.is_set()

    async def wait_until_complete(self, timeout: int | None = None):
        """
        Helper method enabling callers to wait until the response has been completely fetched
        from routeros.
        """
        coro = self.completion_event.wait()
        if timeout:
            coro = asyncio.wait_for(coro, timeout)
        return await coro

    def update_with_sentence(self, sentence: ResponseSentence):
        """
        Updates the response with new data obtained from a sentence received from routeros.
        """
        if self.is_complete():
            raise RuntimeError(f"response is complete")
        self.sentences.append(sentence)
        if sentence.type == "!trap":
            self.status = ResponseStatus.Error
        elif sentence.type == "!done":
            if self.status == ResponseStatus.InProgress:
                self.status = ResponseStatus.Success
            self.completion_event.set()

    def cancel(self):
        """
        'Cancels' an in-progress response by adding a fake sentence with an error indicating the response was cancelled.

        The response is then handled as a 'completed' response, and invokes error logic associated with a failed response

        NOTE: Despite the name, this doesn't invoke the 'cancel' routeros api - it cancels the request client-side
        """
        if self.is_complete():
            raise RuntimeError(f"rresponse is complete")
        sentence = ResponseSentence(type="!trap")
        sentence.attributes["message"] = f"response cancelled"
        self.update_with_sentence(sentence)

    def raise_for_error(self):
        """
        Helper method to raise an exception if a completed response returned an error.
        """
        if not self.is_complete():
            raise RuntimeError(f"response in progress")
        if self.status != ResponseStatus.Error:
            return
        raise ResponseError(response=self)

    def get_data(self) -> list[dict]:
        """
        Helper method to return data from all '!re' sentences in a successful request
        """
        if not self.is_complete():
            raise RuntimeError(f"response in progress")
        if not self.status == ResponseStatus.Success:
            raise RuntimeError(f"response not success")
        data = []
        for sentence in self.sentences:
            if sentence.type != "!re":
                continue
            data.append(dict(sentence.attributes))
        return data

    def get_error_data(self) -> list[dict]:
        """
        Helper method to return the data from all '!trap' sentences in a failed request
        """
        if not self.is_complete():
            raise RuntimeError(f"response in progress")
        if not self.status == ResponseStatus.Error:
            raise RuntimeError(f"response not error")
        data = []
        for sentence in self.sentences:
            if sentence.type != "!trap":
                continue
            data.append(dict(sentence.attributes))
        return data


class ResponseError(Exception):
    """
    Represents a response error sent from routeros
    """

    # the response producing this exception
    response: Response

    def __init__(self, response: Response):
        messages = []
        for error_data in response.get_error_data():
            message = error_data.get("message", "unknown error")
            messages.append(message)
        super().__init__(f"response error: {messages}")
        self.response = response


# convenience type holding the output of `asyncio.open_connection`
Stream = tuple[asyncio.StreamReader, asyncio.StreamWriter]


class Connection:
    """
    Low-level interface managing the socket connection to routeros.
    """

    # background tasks that run while the connection is active (e.g., reading data, checking idle timers)
    background_tasks: list[asyncio.Task]
    # event that's signalled when the connection is closed
    closed_event: asyncio.Event
    # the connection host
    host: str
    # time since activity was last detected
    idle_since: datetime.datetime
    # when timeout exceeded, close the open socket
    idle_timeout: datetime.timedelta
    # the password to supply to routeros' /login api
    password: str
    # in-progress responses being actively worked on by the background task
    responses: dict[str, Response]
    # the stream (reader, writer) connected via socket to the host
    stream: Stream | None
    # a lock that ensures that protects the critical path around socket opening/closing vs. reuse
    stream_lock: asyncio.Lock
    # the username to supply to routeros' /login api
    username: str

    def __init__(self, host: str, password: str, username: str):
        self.background_tasks = []
        self.closed_event = asyncio.Event()
        self.closed_event.set()
        self.host = host
        self.idle_since = datetime.datetime.now()
        self.idle_timeout = datetime.timedelta(seconds=10)
        self.password = password
        self.responses = {}
        self.stream = None
        self.stream_lock = asyncio.Lock()
        self.username = username

    async def run_background_idle_monitor(self):
        """
        Run loop that checks whether the connection is idle.  If it is, the connection is closed.
        """
        while not self.closed_event.is_set():
            if not await self.is_idle():
                await asyncio.sleep(1.0)
                continue
            logger.debug(f"idle socket detected")
            # use `create_task` here - this coroutine is awaited as part of `close()`
            asyncio.create_task(self.close())
            # break to prevent several `close()` coroutines from being scheduled
            break

    async def run_background_read(self):
        """
        Run loop that reads from the connected data socket while the connection is active
        """
        while not self.closed_event.is_set():
            await self.read()

    async def is_idle(self) -> bool:
        """
        Returns true when the socket has been idle longer than `idle_timeout`
        """
        now = datetime.datetime.now()
        return (now - self.idle_since) >= self.idle_timeout

    async def read(self):
        """
        Reads sentences from the connected socket.

        Notifies waiters when a response has been fully read.
        """
        if self.stream:
            reader, _ = self.stream
            sentence = await read_sentence(reader)
            if sentence is None:
                return
            self.idle_since = datetime.datetime.now()
            tag = sentence.api_attributes.get(".tag")
            if tag is None:
                raise RuntimeError(f"tag not found")
            response = self.responses[tag]
            logger.debug(f"receive response sentence ({tag}) {sentence.type}")
            response.update_with_sentence(sentence)
            if response.is_complete():
                logger.debug(f"receive response ({tag})")
                self.responses.pop(tag)

    async def open(self) -> Stream:
        """
        Ensures that the client creates a single socket connection to routeros
        through which multiple api calls can be made.

        Performs other initialization that requires a connection to be made -
        like authentication and background response processing tasks.
        """
        # use a lock to ensure that only one connection attempt is made
        async with self.stream_lock:
            old_idle_since = self.idle_since
            self.idle_since = datetime.datetime.now()
            logger.debug(f"idle since: {old_idle_since} -> {self.idle_since}")

            if not self.stream:
                logger.debug(f"opening connection")

                # connect
                self.stream = await asyncio.open_connection(host=self.host, port=8728)

                # clear the `closed_event` signal since the connection is re-opening
                self.closed_event.clear()

                # start background tasks
                self.background_tasks = [
                    asyncio.create_task(self.run_background_idle_monitor()),
                    asyncio.create_task(self.run_background_read()),
                ]

                # authenticate with routeros
                try:
                    response = await self.send(
                        "/login",
                        f"=name={self.username}",
                        f"=password={self.password}",
                        stream=self.stream,
                    )
                    response.raise_for_error()
                except Exception as e:
                    asyncio.create_task(self.close())
                    raise e

            return self.stream

    async def close(self):
        """
        Closes an open connection.

        Cleans up anything that might have been initialized from a call to `open()`
        """
        async with self.stream_lock:
            # do nothing if `close()` has already been called
            if self.closed_event.is_set():
                return

            logger.debug(f"closing connection")

            # setting this event allows background tasks to gracefully shut down
            self.closed_event.set()

            # ensure background tasks are no longer running
            while self.background_tasks:
                task = self.background_tasks.pop()
                await task

            # close the stream
            if self.stream:
                _, writer = self.stream
                writer.close()
                await writer.wait_closed()
                self.stream = None

            # cancel all in-progress responses
            while self.responses:
                _, response = self.responses.popitem()
                response.cancel()

    async def send(self, *sentence: str, stream: Stream | None = None):
        """
        Sends a request to routeros.

        The 'stream' option should *not* be provided unless this is being called from within `open()`.
        """
        stream = stream or await self.open()
        request = Request(list(sentence))
        response = Response(request)
        self.responses[response.tag] = response
        logger.debug(f"send request ({request.tag}) {request.words[0]}")
        _, writer = stream
        await write_sentence(writer, request.get_sentence())
        await response.wait_until_complete()
        return response


class Client:
    # the underlying connection through which apis will communicate
    connection: Connection

    def __init__(self, *, host: str, password: str, username: str):
        self.connection = Connection(host=host, password=password, username=username)

    async def add_ip_dns_record(self, ip_dns_record: IpDnsRecord):
        """
        Adds an IPV4 dns record to routeros
        """
        data = ip_dns_record.model_dump(
            by_alias=True, exclude_none=True, exclude={"id"}
        )
        words = ["/ip/dns/static/add", *to_attribute_words(data)]
        response = await self.connection.send(*words)
        response.raise_for_error()

    async def delete_ip_dns_record(self, ip_dns_record: IpDnsRecord):
        """
        Deletes an IPV4 dns records registered with routeros by id
        """
        words = ["/ip/dns/static/remove", f"=numbers={ip_dns_record.id}"]
        response = await self.connection.send(*words)
        response.raise_for_error()

    async def list_ip_dns_records(self) -> list[IpDnsRecord]:
        """
        Lists IPV4 dns records registered with routeros
        """
        response = await self.connection.send("/ip/dns/static/print", "=detail=")
        response.raise_for_error()
        model_cls: pydantic.TypeAdapter[IpDnsRecord] = pydantic.TypeAdapter(IpDnsRecord)
        raw = response.get_data()
        for item in raw:
            item.setdefault("type", "A")
        data = list(map(model_cls.validate_python, raw))
        return data


async def write_sentence(writer: asyncio.StreamWriter, sentence: list[str]):
    """
    Helper method to write a setence to a socket connected to the RouterOS API.
    """
    # add an empty write to the sentence to indicate to routeros that the sentence is finished
    for word in sentence:
        await write_word(writer, word)


async def write_word(writer: asyncio.StreamWriter, word: str):
    """
    Writes a word to a socket connected to the RouterOS API.

    Reference: https://help.mikrotik.com/docs/display/ROS/API#API-APIwords
    """
    # encode the length
    # NOTE: this mask is used in `read_word`
    if len(word) <= 0x7F:
        mask = 0x00
        num_bytes = 1
    elif len(word) <= 0x3FFF:
        mask = 0x8000
        num_bytes = 2
    elif len(word) <= 0x1FFFFF:
        mask = 0xC00000
        num_bytes = 3
    elif len(word) <= 0xFFFFFFF:
        mask = 0xE0000000
        num_bytes = 4
    else:
        mask = 0xF000000000
        num_bytes = 5
    encoded_length = (mask | len(word)).to_bytes(num_bytes, "big")

    # assemble the word
    data = encoded_length + word.encode()

    writer.write(data)
    await writer.drain()


async def read_sentence(reader: asyncio.StreamReader) -> ResponseSentence | None:
    """
    Reads a sequence of words from a socket connected to the RouterOS API.

    Returns a sentence object representing the read data
    """
    sentence: ResponseSentence | None = None

    while True:
        word = await read_word(reader)
        if word == "":
            break
        elif word.startswith("!"):
            # read a sentence type - signals the start of a new sentence
            sentence = ResponseSentence(type=word)
        elif sentence and word.startswith("="):
            # read an attribute key/value pair
            parts = word[1:].split("=")
            key, value = parts[0], "=".join(parts[1:])
            sentence.attributes[key] = value
        elif sentence and word.startswith("."):
            # read an api attribute key/value pair
            key, tag = word.split("=")
            sentence.api_attributes[key] = tag
    return sentence


async def read_word(reader: asyncio.StreamReader) -> str:
    """
    Reads a word from a socket connected to the RouterOS API.

    Reference: https://help.mikrotik.com/docs/display/ROS/API#API-APIwords
    """
    # read encoded length
    # NOTE: these values are obtained from the masks in `write_word`
    try:
        length = await asyncio.wait_for(reader.readexactly(1), timeout=1)
    except asyncio.TimeoutError:
        return ""
    header = ord(length)
    if header & 0xF0 == 0xF0:
        num_bytes = 4
    elif header & 0xE0 == 0xE0:
        num_bytes = 3
    elif header & 0xC0 == 0xC0:
        num_bytes = 2
    elif header & 0x80 == 0x80:
        num_bytes = 1
    else:
        num_bytes = 0
    length = ord(length + (await reader.readexactly(num_bytes)))

    # read exactly 'length' bytes to obtain data
    data = await reader.readexactly(length)
    return data.decode()


def to_word_value(value: Any) -> str:
    """
    Helper method that converts a value into a valid routeros api word value
    """
    # NOTE: str(None) is 'None', should be ''
    if value is None:
        return ""
    # NOTE: 'str(bool) -> 'True/False', should be 'true/false'
    if isinstance(value, bool):
        return "true" if value else "false"
    return str(value)


def to_attribute_words(val: dict) -> list[str]:
    """
    Helper method that translates a set of attributes into valid routeros api words
    """
    words = []
    for key, value in val.items():
        words.append(f"={key}={to_word_value(value)}")
    return words


def to_api_attribute_words(val: dict) -> list[str]:
    """
    Helper method that translates a set of api attributes into valid routeros api words

    Ensures that each key is prefixed with '.'
    """
    words = []
    for key, value in val.items():
        if not key.startswith("."):
            raise ValueError(f"key missing . prefix: {key}")
        words.append(f"{key}={to_word_value(value)}")
    return words

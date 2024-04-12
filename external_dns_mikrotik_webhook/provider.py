import contextlib
import datetime
import enum
import logging
import re

import pydantic
from pykrotik import Client as MikrotikClient
from pykrotik import IpDnsRecord, IpDnsRecordType

logger = logging.getLogger(__name__)


class BaseModel(pydantic.BaseModel):
    """
    Defines a base model from which other models can be defined
    """

    # config that:
    #  - allows models to be populated by alias and field name
    model_config = pydantic.ConfigDict(populate_by_name=True)


class DomainFilter(BaseModel):
    """
    Defines a set of rules that external-dns will use to include/exclude dns records for processing.

    Reference: https://github.com/kubernetes-sigs/external-dns/blob/master/endpoint/domain_filter.go#L56
    """

    # list of domains to include
    include: list[str] | None = None
    # list of domains to exclude
    exclude: list[str] | None = None
    # regex to match domains to include
    regex_include: re.Pattern | None = pydantic.Field(
        default=None, alias=str("regexInclude")
    )
    # regex to match domains to exclude
    regex_exclude: re.Pattern | None = pydantic.Field(
        default=None, alias=str("regexExclude")
    )


class RecordType(str, enum.Enum):
    """
    Record types that external-dns can process
    """

    A = "A"
    AAAA = "AAAA"
    CNAME = "CNAME"
    TXT = "TXT"
    SRV = "SRV"
    NS = "NS"
    PTR = "PTR"
    MX = "MX"
    NAPTR = "NAPTR"


class ProviderSpecificItem(BaseModel):
    """
    An entry within a list of provider specific data provided alongside an `Endpoint`.
    """

    # the name of the item
    name: str
    # the value of the item
    value: str


class Endpoint(BaseModel):
    """
    Represents a dns record as known to external-dns.

    Reference: https://github.com/kubernetes-sigs/external-dns/blob/master/endpoint/endpoint.go#L177
    """

    # the name of the dns record
    dns_name: str = pydantic.Field(alias=str("dnsName"))
    # the targets the dns record points to
    targets: list[str]
    # the type of record
    record_type: RecordType = pydantic.Field(alias=str("recordType"))
    # a set identifier - used to disambiguate records that have identical name + type + target
    set_identifier: str | None = pydantic.Field(
        default=None, alias=str("setIdentifier")
    )
    # a ttl for the record - described in number of seconds (int)
    record_ttl: int | None = pydantic.Field(default=None, alias=str("recordTTL"))
    # labels attached to the dns record
    labels: dict[str, str] | None = pydantic.Field(default=None)
    # provider specific metadata, if needed
    provider_specific: list[ProviderSpecificItem] | None = pydantic.Field(
        default=None, alias=str("providerSpecific")
    )


class Changes(BaseModel):
    """
    Represents a set of changes needed by the external-dns operator that need to be performed by the provider.

    Reference: https://github.com/kubernetes-sigs/external-dns/blob/master/plan/plan.go#L34
    """

    # dns records to create
    create: list[Endpoint] | None = pydantic.Field(default=None, alias=str("Create"))
    # old versions of changed dns records
    update_old: list[Endpoint] | None = pydantic.Field(
        default=None, alias=str("UpdateOld")
    )
    # new versions of changed dns records
    update_new: list[Endpoint] | None = pydantic.Field(
        default=None, alias=str("UpdateNew")
    )
    # dns records to delete
    delete: list[Endpoint] | None = pydantic.Field(default=None, alias=str("Delete"))


class RecordMap:
    """
    Class that assists with searching for dns records when `Provider.apply_changes` is called.
    """

    # map of record name to list of records
    data: dict[str, list[IpDnsRecord]]

    def __init__(self, records: list[IpDnsRecord]):
        self.data = {}
        for record in records:
            self.data.setdefault(record.name, []).append(record)

    def find(self, endpoint: Endpoint, target: str) -> IpDnsRecord | None:
        """
        Given an endpoint and target - finds a matching `IpDnsRecord` if one exists.

        Return None if record not found.
        """
        for item in self.data.get(endpoint.dns_name, []):
            if item.type == IpDnsRecordType.A and item.address == target:
                return item
            elif item.type == IpDnsRecordType.CNAME and item.cname == target:
                return item
            elif item.type == IpDnsRecordType.TXT and item.text == target:
                return item
        return None


class Provider:
    # a client capable of connecting to a routeros instance
    client: MikrotikClient
    # a user-configurable domain filter that can optionally include/exclude records from particular domains
    doman_filter: DomainFilter

    def __init__(
        self, client: MikrotikClient, domain_filter: DomainFilter | None = None
    ):
        self.client = client
        self.domain_filter = domain_filter or DomainFilter()

    async def adjust_endpoints(self, endpoints: list[Endpoint]) -> list[Endpoint]:
        """
        Adjusts an incoming set of endpoints to match any routeros specific requirements and returns them.
        """
        logger.info(f"adjust endpoints called with {len(endpoints)} endpoints")
        return endpoints

    async def apply_changes(self, changes: Changes):
        """
        Applies a set of changes request by external-dns to the dns records stored in routeros
        """
        creates = changes.create or []
        update_olds = changes.update_old or []
        update_news = changes.update_new or []
        updates = list(zip(update_olds, update_news))
        deletes = changes.delete or []

        logger.info(
            f"apply changes called with {len(creates)} creates, {len(deletes)} deletes, {len(updates)} updates"
        )

        records = await self.client.list_ip_dns_records()
        logger.debug(f"listed {len(records)} records from routeros")
        record_map = RecordMap(records)

        @contextlib.contextmanager
        def log_action(action: str, endpoint: Endpoint, target: str):
            try:
                logger.debug(
                    f"{action} record: {endpoint.record_type} {endpoint.dns_name} {target}"
                )
                yield
            except Exception as e:
                logger.exception(
                    f"{action} record: {endpoint.record_type} {endpoint.dns_name} {target}"
                )

        # handle new records
        for endpoint in creates:
            for target in endpoint.targets:
                with log_action("create", endpoint, target):
                    record = to_routeros_record(endpoint, target)
                    await self.client.add_ip_dns_record(record)

        # handle deleted records
        for endpoint in deletes:
            for target in endpoint.targets:
                with log_action("delete", endpoint, target):
                    record = record_map.find(endpoint, target)
                    if not record:
                        logger.debug(f"routeros record not found")
                        continue
                    await self.client.delete_ip_dns_record(record)

        # handle updated records
        for old_endpoint, endpoint in updates:
            old_targets = set(old_endpoint.targets)
            new_targets = set(endpoint.targets)

            # handle deleted targets for existing records
            for target in old_targets - new_targets:
                with log_action("delete updated", endpoint, target):
                    record = record_map.find(endpoint, target)
                    if not record:
                        logger.debug(f"routeros record not found")
                        continue
                    await self.client.delete_ip_dns_record(record)

            # handle created targets for existing records
            for target in new_targets - old_targets:
                with log_action("create updated", endpoint, target):
                    record = to_routeros_record(endpoint, target)
                    await self.client.add_ip_dns_record(record)

    async def get_domain_filter(self) -> DomainFilter:
        """
        Defines a set of DNS filters on which external-dns should act.
        """
        logger.info(f"get domain filter called")
        return self.domain_filter

    async def list_records(self) -> list[Endpoint]:
        """
        Lists all DNS records
        """
        logger.info(f"list records called")
        routeros_records = await self.client.list_ip_dns_records()
        endpoints = []
        for routeros_record in routeros_records:
            try:
                endpoint = to_external_dns_endpoint(routeros_record)
            except NotImplementedError:
                continue
            endpoints.append(endpoint)
        return endpoints


def to_routeros_record(endpoint: Endpoint, target: str) -> IpDnsRecord:
    """
    Helper method to convert an external-dns endpoint into a routeros dns record
    """
    kwargs: dict = dict(
        disabled=False,
        dynamic=False,
        match_subdomain=target.startswith("*."),
        name=endpoint.dns_name,
        ttl=to_routeros_ttl(endpoint.record_ttl or 60 * 60 * 24),
    )

    if endpoint.record_type == RecordType.A:
        return IpDnsRecord(address=target, type=IpDnsRecordType.A, **kwargs)
    if endpoint.record_type == RecordType.CNAME:
        return IpDnsRecord(cname=target, type=IpDnsRecordType.CNAME, **kwargs)
    elif endpoint.record_type == RecordType.TXT:
        return IpDnsRecord(text=target, type=IpDnsRecordType.TXT, **kwargs)
    else:
        raise NotImplementedError()


def to_external_dns_endpoint(record: IpDnsRecord) -> Endpoint:
    """
    Helper method to convert a routeros dns record to an external-dns endpoint.
    """
    if record.type == IpDnsRecordType.A:
        targets = [record.address]
    elif record.type == IpDnsRecordType.CNAME:
        targets = [record.cname]
    elif record.type == IpDnsRecordType.TXT:
        targets = [record.text]
    else:
        raise NotImplementedError()
    targets = list(map(str, targets))

    endpoint = Endpoint(
        dns_name=record.name,
        labels={},
        provider_specific=[],
        record_ttl=to_external_dns_ttl(record.ttl),
        record_type=RecordType(record.type.value),
        set_identifier=None,
        targets=targets,
    )
    return endpoint


def to_routeros_ttl(external_dns_ttl: int) -> str:
    """
    Helper method to convert a ttl field from an external-dns format (694861)
    to a routeros format ('1w1d1h1m1s').
    """
    td = datetime.timedelta(seconds=external_dns_ttl)
    w = int(td.days / 7)
    d = int(td.days % 7)
    h = int(td.seconds / (60 * 60))
    m = int((td.seconds % (60 * 60)) / 60)
    s = int(td.seconds % 60)
    return f"{w}w{d}d{h}h{m}m{s}s"


def to_external_dns_ttl(mikrotik_ttl: str) -> int:
    """
    Helper method to convert a ttl field from a routeros format ('1w1d1h1m1s') to
    an external-dns format (694861).
    """
    # ttl = 1w1d1h1m1s
    # create a regex for each 'segment' of the ttl (e.g., named group 'd' -> '1d')
    regex = re.compile(
        r"(?P<w>[0-9]+w)?(?P<d>[0-9]+d)?(?P<h>[0-9]+h)?(?P<m>[0-9]+m)?(?P<s>[0-9]+s)?"
    )
    match = regex.match(mikrotik_ttl)
    if not match:
        raise ValueError(mikrotik_ttl)

    # convert potentially missing string values into guaranteed int values
    # (e.g., '1d' -> 1, a missing 'w' will become '0w')
    parts = {}
    for key in ["w", "d", "h", "m", "s"]:
        str_val = match.group(key) or f"0{key}"
        val = int(str_val.replace(key, ""))
        parts[key] = val

    # calculate the total number of seconds
    total = parts["s"]
    total += parts["m"] * 60
    total += parts["h"] * 60 * 60
    total += parts["d"] * 60 * 60 * 24
    total += parts["w"] * 60 * 60 * 24 * 7

    return total

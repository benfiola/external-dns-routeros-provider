import asyncio
import datetime
import json
import pathlib

import pytest

import external_dns_mikrotik_webhook.mikrotik as mikrotik


async def test_request_get_sentence_includes_tag():
    request = mikrotik.Request(["/asdf"])
    assert f".tag={request.tag}" in request.get_sentence()


async def test_request_get_sentence_ends_with_empty_string():
    request = mikrotik.Request(["/asdf"])
    assert request.get_sentence()[-1] == ""


async def test_request_wait_for_complete_waits_for_event_signal():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    # will timeout because response is not complete
    with pytest.raises(asyncio.TimeoutError):
        await asyncio.wait_for(response.wait_until_complete(), timeout=0.5)
    response.completion_event.set()
    # will not timeout because response is complete (but wrap in timeout just in case)
    await asyncio.wait_for(response.wait_until_complete(), timeout=0.5)


async def test_response_update_with_sentence_adds_sentence():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    response_sentence = mikrotik.ResponseSentence("!test")
    response.update_with_sentence(response_sentence)
    assert response_sentence in response.sentences


async def test_response_update_with_sentence_sets_error_status_on_trap_sentence():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    assert response.status == mikrotik.ResponseStatus.InProgress
    response_sentence = mikrotik.ResponseSentence("!trap")
    response.update_with_sentence(response_sentence)
    assert response.status == mikrotik.ResponseStatus.Error


async def test_response_update_with_sentence_sets_success_status_on_done_sentence():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    assert response.status == mikrotik.ResponseStatus.InProgress
    response_sentence = mikrotik.ResponseSentence("!done")
    response.update_with_sentence(response_sentence)
    assert response.status == mikrotik.ResponseStatus.Success


async def test_response_update_with_sentence_preserves_error_status_on_done_sentence():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    assert response.status == mikrotik.ResponseStatus.InProgress
    response_sentence = mikrotik.ResponseSentence("!trap")
    response.update_with_sentence(response_sentence)
    response_sentence = mikrotik.ResponseSentence("!done")
    response.update_with_sentence(response_sentence)
    assert response.status == mikrotik.ResponseStatus.Error


async def test_response_update_with_sentence_sets_event_on_done_sentence():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    assert not response.completion_event.is_set()
    response_sentence = mikrotik.ResponseSentence("!done")
    response.update_with_sentence(response_sentence)
    assert response.completion_event.is_set()


async def test_response_update_with_sentence_disallows_update_when_completed():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    response_sentence = mikrotik.ResponseSentence("!done")
    response.update_with_sentence(response_sentence)
    with pytest.raises(RuntimeError) as e:
        response.update_with_sentence(response_sentence)
    assert "response is complete" in str(e.value)


async def test_response_cancel_raises_exception_if_complete():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    response.completion_event.set()
    with pytest.raises(RuntimeError) as e:
        response.cancel()
    assert "response is complete" in str(e.value)


async def test_response_cancel_adds_trap_sentence():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    response.cancel()
    found = None
    for sentence in response.sentences:
        if sentence.type != "!trap":
            continue
        message = sentence.attributes.get("message", "")
        if message == "response cancelled":
            found = message
    assert found is not None


async def test_response_raise_for_error_raises_exception_if_request_not_complete():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    with pytest.raises(RuntimeError) as e:
        response.raise_for_error()
    assert "response in progress" in str(e.value)


async def test_response_raise_for_error_noop_if_request_status_success():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    response.completion_event.set()
    response.raise_for_error()


async def test_response_raise_for_error_raises_exception_if_request_status_error():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    response.status = mikrotik.ResponseStatus.Error
    response.completion_event.set()
    with pytest.raises(mikrotik.ResponseError) as e:
        response.raise_for_error()


async def test_response_get_data_raises_exception_if_request_not_complete():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    with pytest.raises(RuntimeError) as e:
        response.get_data()
    assert f"response in progress" in str(e.value)


async def test_response_get_data_raises_exception_if_request_status_not_success():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    response.completion_event.set()
    with pytest.raises(RuntimeError) as e:
        response.get_data()
    assert f"response not success" in str(e.value)


async def test_response_get_data_returns_re_sentence_attributes():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    response.completion_event.set()
    response.status = mikrotik.ResponseStatus.Success
    sentence = mikrotik.ResponseSentence("!re")
    sentence.attributes["a"] = "1"
    other_sentence = mikrotik.ResponseSentence("!done")
    other_sentence.attributes["b"] = "2"
    response.sentences.extend([sentence, other_sentence])
    data = response.get_data()
    assert sentence.attributes in data
    assert other_sentence.attributes not in data


async def test_response_get_error_data_raises_exception_if_request_not_complete():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    with pytest.raises(RuntimeError) as e:
        response.get_error_data()
    assert f"response in progress" in str(e.value)


async def test_response_get_data_raises_exception_if_request_status_not_error():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    response.completion_event.set()
    with pytest.raises(RuntimeError) as e:
        response.get_error_data()
    assert f"response not error" in str(e.value)


async def test_response_get_error_data_returns_trap_sentence_attributes():
    request = mikrotik.Request(["/asdf"])
    response = mikrotik.Response(request)
    response.completion_event.set()
    response.status = mikrotik.ResponseStatus.Error
    sentence = mikrotik.ResponseSentence("!trap")
    sentence.attributes["a"] = "1"
    other_sentence = mikrotik.ResponseSentence("!re")
    other_sentence.attributes["b"] = "2"
    response.sentences.extend([sentence, other_sentence])
    data = response.get_error_data()
    assert sentence.attributes in data
    assert other_sentence.attributes not in data


def test_to_attribute_words_produces_correct_output():
    raw = {"str": "a", "int": 1, "bool": True, "none": None}
    expecteds = {"str": "a", "int": "1", "bool": "true", "none": ""}
    words = mikrotik.to_attribute_words(raw)
    for word in words:
        parts = word.split("=")
        key = parts[1]
        value = "" if len(parts) < 3 else parts[2]
        assert value == expecteds[key]


def test_to_api_attribute_words_produces_correct_output():
    raw = {".id": "*1"}
    expecteds = raw
    words = mikrotik.to_api_attribute_words(raw)
    for word in words:
        parts = word.split("=")
        key = parts[0]
        value = "" if len(parts) < 2 else parts[1]
        assert value == expecteds[key]


def test_to_api_attribute_fails_if_key_wrong_format():
    raw = {"id": "*1"}
    with pytest.raises(ValueError) as e:
        mikrotik.to_api_attribute_words(raw)
    assert "key missing . prefix: id" in str(e.value)


local_folder = pathlib.Path(__file__).parent.parent.joinpath("local")


def client_from_json() -> mikrotik.Client:
    settings_file = local_folder.joinpath("test_mikrotik_integration.json")
    settings = json.loads(settings_file.read_text())
    client = mikrotik.Client(
        host=settings["host"],
        password=settings["password"],
        username=settings["username"],
    )
    return client


def integration_test(func):
    return pytest.mark.skip()(func)


@integration_test
async def test_integration_client_list_ip_dns():
    client = client_from_json()
    data = await client.list_ip_dns_records()
    assert data is not None


@integration_test
async def test_integration_client_idle_monitor():
    client = client_from_json()
    client.connection.idle_timeout = datetime.timedelta(seconds=2)
    await client.list_ip_dns_records()
    assert client.connection.stream is not None
    await asyncio.sleep(client.connection.idle_timeout.total_seconds() + 1)
    assert client.connection.stream is None

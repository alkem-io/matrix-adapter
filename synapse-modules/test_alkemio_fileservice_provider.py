# Copyright 2026 Alkemio Foundation
# SPDX-License-Identifier: EUPL-1.2

"""
Unit tests for FileServiceStorageProvider (013-matrix-media-file-service).

Run inside the Synapse virtualenv (needs twisted + treq + synapse importable):

    pip install pytest twisted treq service-identity
    pytest test_alkemio_fileservice_provider.py

The treq HTTP layer and Synapse's threadpool/deferred helpers are monkeypatched,
so no live file-service or reactor is required. These run in the
matrixdotorg/synapse image where twisted/treq/synapse are importable. On a bare
host without Synapse, drop a local (untracked) `conftest.py` beside this file that
injects lightweight `sys.modules` shims for the handful of Synapse symbols the
module imports at collection time — the production import path stays untouched.
"""

import asyncio
import types

import pytest

import alkemio_fileservice_provider as mod
from alkemio_fileservice_provider import (
    FileServiceStorageProvider,
    _CircuitBreaker,
    _CircuitOpenError,
    _ConsumerSink,
    _DrainAndAbort,
    _FileServiceResponder,
    _CLOSED,
    _OPEN,
    _HALF_OPEN,
)


# --- test doubles ----------------------------------------------------------


class FakeFileInfo:
    # Mirrors the real Synapse synapse.media._base.FileInfo surface the provider
    # touches: file_id + the routing flags. There is deliberately NO `upload_path`
    # attribute — real FileInfo has none, and the provider derives the on-disk
    # path from media_store_path + the `path` arg instead.
    def __init__(self, file_id, server_name=None, thumbnail=None, url_cache=None):
        self.file_id = file_id
        self.server_name = server_name
        self.thumbnail = thumbnail
        self.url_cache = url_cache


class FakeTransport:
    """Stand-in for Twisted's response-body TransportProxyProducer."""

    def __init__(self):
        self.stopped = False

    def stopProducing(self):
        self.stopped = True


class FakeResponse:
    def __init__(self, code, json_body=None, body=b""):
        self.code = code
        self._json = json_body
        self._body = body
        self.delivered_to = None
        self.transport = None

    def content(self):
        return _aval(self._body)

    def deliverBody(self, protocol):
        # Mimic Twisted: hand the body protocol a producer transport so its
        # makeConnection (producer registration / abort) actually runs.
        self.delivered_to = protocol
        self.transport = FakeTransport()
        protocol.makeConnection(self.transport)


class FakeConsumer:
    """Stand-in for a Synapse media IConsumer on the read path."""

    def __init__(self, reject_producer=False):
        self.data = bytearray()
        self.producer = None
        self.streaming = None
        self.unregistered = False
        self._reject_producer = reject_producer

    def registerProducer(self, producer, streaming):
        if self._reject_producer:
            raise RuntimeError("consumer refuses a producer")
        self.producer = producer
        self.streaming = streaming

    def unregisterProducer(self):
        self.unregistered = True

    def write(self, data):
        self.data += data


class FakeFile:
    """Stand-in for a streamed file handle (asserts it is streamed, not buffered)."""

    def __init__(self, data=b"RAWBYTES"):
        self._data = data
        self.closed = False

    def read(self, n=-1):
        return self._data

    def close(self):
        self.closed = True


async def _aval(x):
    return x


def _run(coro):
    loop = asyncio.new_event_loop()
    try:
        return loop.run_until_complete(coro)
    finally:
        loop.close()


def _make_provider(**config_overrides):
    base = {
        "file_service_url": "http://file-service:4003/",
        "matrix_media_bucket_id": "00000000-0000-0000-0000-0000000000ff",
    }
    base.update(config_overrides)
    cfg = FileServiceStorageProvider.parse_config(base)
    hs = types.SimpleNamespace(
        get_reactor=lambda: object(),
        config=types.SimpleNamespace(
            media=types.SimpleNamespace(media_store_path="/data/media_store")
        ),
    )
    return FileServiceStorageProvider(hs, cfg)


@pytest.fixture(autouse=True)
def _patch_async_helpers(monkeypatch):
    # make_deferred_yieldable / defer_to_thread just pass the awaitable through.
    monkeypatch.setattr(mod, "make_deferred_yieldable", lambda d: d)

    async def _defer_to_thread(reactor, fn, *args):
        return fn(*args)

    monkeypatch.setattr(mod, "defer_to_thread", _defer_to_thread)


# --- parse_config ----------------------------------------------------------


def test_parse_config_requires_url():
    with pytest.raises(ValueError):
        FileServiceStorageProvider.parse_config({"matrix_media_bucket_id": "x"})


def test_parse_config_requires_bucket():
    with pytest.raises(ValueError):
        FileServiceStorageProvider.parse_config(
            {"file_service_url": "http://file-service:4003"}
        )


def test_parse_config_strips_trailing_slash():
    cfg = FileServiceStorageProvider.parse_config(
        {"file_service_url": "http://fs:4003/", "matrix_media_bucket_id": "b"}
    )
    assert cfg["file_service_url"] == "http://fs:4003"


def test_parse_config_present_but_null_falls_back_to_default():
    # A YAML `timeout_s:` with no value parses to None; it must fall back to the
    # default, NOT blow up in float(None)/int(None).
    cfg = FileServiceStorageProvider.parse_config(
        {
            "file_service_url": "http://fs:4003",
            "matrix_media_bucket_id": "b",
            "timeout_s": None,
            "store_timeout_s": None,
            "cb_fail_threshold": None,
            "cb_reset_timeout_s": None,
        }
    )
    assert cfg["timeout_s"] == mod.DEFAULT_TIMEOUT_S
    assert cfg["store_timeout_s"] == mod.DEFAULT_STORE_TIMEOUT_S
    assert cfg["cb_fail_threshold"] == mod.DEFAULT_CB_FAIL_THRESHOLD
    assert cfg["cb_reset_timeout_s"] == mod.DEFAULT_CB_RESET_TIMEOUT_S


@pytest.mark.parametrize(
    "overrides",
    [
        {"cb_fail_threshold": 0},
        {"cb_fail_threshold": -1},
        {"timeout_s": 0},
        {"store_timeout_s": -5},
        {"cb_reset_timeout_s": 0},
        {"timeout_s": "not-a-number"},
    ],
)
def test_parse_config_rejects_non_positive_or_bad_tuning(overrides):
    base = {
        "file_service_url": "http://fs:4003",
        "matrix_media_bucket_id": "b",
    }
    base.update(overrides)
    with pytest.raises(ValueError):
        FileServiceStorageProvider.parse_config(base)


# --- routing ---------------------------------------------------------------


def test_routes_only_user_uploads():
    p = FileServiceStorageProvider
    assert p._is_user_upload(FakeFileInfo("m1")) is True
    # remote media
    assert p._is_user_upload(FakeFileInfo("m2", server_name="other.host")) is False
    # thumbnail
    assert p._is_user_upload(FakeFileInfo("m3", thumbnail=object())) is False
    # url-cache preview
    assert p._is_user_upload(FakeFileInfo("m4", url_cache=1)) is False


def test_store_skips_non_user_upload(monkeypatch):
    prov = _make_provider()
    called = {"post": False}
    monkeypatch.setattr(
        mod.treq, "post", lambda *a, **k: called.__setitem__("post", True)
    )
    _run(prov.store_file("remote_content/x", FakeFileInfo("m", server_name="h")))
    assert called["post"] is False


# --- store_file ------------------------------------------------------------


def test_store_posts_verbatim_multipart_streamed(monkeypatch):
    prov = _make_provider()
    captured = {}

    def fake_post(url, files=None, data=None, **kw):
        captured["url"] = url
        captured["files"] = files
        captured["data"] = data
        return _aval(FakeResponse(201, json_body={"id": "doc-1"}))

    opened = []
    fake_file = FakeFile(b"RAWBYTES")

    monkeypatch.setattr(mod.treq, "post", fake_post)
    monkeypatch.setattr(
        mod, "_open_stream", lambda p: opened.append(p) or fake_file
    )

    _run(prov.store_file("local_content/aa/bb/MEDIAID", FakeFileInfo("MEDIAID")))

    # C3 regression guard: the provider must open the local cache file at
    # media_store_path + the `path` arg, NOT a (non-existent) FileInfo attribute.
    assert opened == ["/data/media_store/local_content/aa/bb/MEDIAID"]
    assert captured["url"] == "http://file-service:4003/internal/file"
    assert captured["data"]["externalReference"] == "MEDIAID"
    assert captured["data"]["skipImageProcessing"] == "true"
    assert (
        captured["data"]["storageBucketId"]
        == "00000000-0000-0000-0000-0000000000ff"
    )
    # The body must be the STREAMED file handle, not a fully-buffered bytes blob.
    body = captured["files"]["file"][1]
    assert body is fake_file
    assert not isinstance(body, (bytes, bytearray))
    assert hasattr(body, "read")
    # And the handle is closed once the post completes.
    assert fake_file.closed is True
    assert prov._write_breaker.state == _CLOSED


def test_store_raises_and_trips_write_breaker_on_http_error(monkeypatch):
    prov = _make_provider(cb_fail_threshold=1)
    fake_file = FakeFile(b"x")
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(FakeResponse(500)))
    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)
    with pytest.raises(RuntimeError):
        _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    assert prov._write_breaker.state == _OPEN
    assert fake_file.closed is True


@pytest.mark.parametrize("code", [200, 202, 204, 301, 302])
def test_store_rejects_non_201_success(monkeypatch, code):
    # A 2xx-non-201 or a 3xx is NOT a confirmed durable store (finding 10).
    prov = _make_provider(cb_fail_threshold=1)
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(FakeResponse(code)))
    monkeypatch.setattr(mod, "_open_stream", lambda p: FakeFile(b"x"))
    with pytest.raises(RuntimeError):
        _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    assert prov._write_breaker.state == _OPEN


def test_store_write_outage_does_not_block_reads(monkeypatch):
    # Finding: a store (write) outage MUST NOT open the breaker guarding fetch.
    prov = _make_provider(cb_fail_threshold=1)
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(FakeResponse(500)))
    monkeypatch.setattr(mod, "_open_stream", lambda p: FakeFile(b"x"))
    with pytest.raises(RuntimeError):
        _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    assert prov._write_breaker.state == _OPEN

    # The read breaker is untouched, so fetch still works.
    def fake_get(url, **kw):
        if "by-reference" in url:
            return _aval(FakeResponse(200, json_body={"id": "doc-9"}))
        return _aval(FakeResponse(200, body=b"CONTENT"))

    monkeypatch.setattr(mod.treq, "get", fake_get)
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))
    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is not None
    assert prov._read_breaker.state == _CLOSED


# --- fetch -----------------------------------------------------------------


def test_fetch_global_lookup_then_streams(monkeypatch):
    prov = _make_provider()
    calls = []

    def fake_get(url, **kw):
        calls.append(url)
        if "by-reference" in url:
            return _aval(FakeResponse(200, json_body={"id": "doc-9"}))
        return _aval(FakeResponse(200, body=b"CONTENT"))

    monkeypatch.setattr(mod.treq, "get", fake_get)
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))

    # global lookup: NO bucketId query param
    assert (
        calls[0]
        == "http://file-service:4003/internal/file/by-reference?ref=MEDIAID"
    )
    assert "bucketId" not in calls[0]
    assert calls[1] == "http://file-service:4003/internal/file/doc-9/content"
    assert responder is not None
    # Success is NOT recorded until the body streams to completion (finding 6):
    # the read breaker is still closed but success has not fired.
    assert prov._read_breaker.state == _CLOSED


def test_fetch_returns_none_on_real_404_miss_neutral(monkeypatch):
    # Drive the actual by-reference 404 branch and assert it is NEUTRAL: a
    # pre-existing (below-threshold) failure count must survive the miss.
    prov = _make_provider()
    prov._read_breaker.on_failure()  # 1 real failure recorded, still closed
    assert prov._read_breaker._failures == 1

    calls = []

    def fake_get(url, **kw):
        calls.append(url)
        return _aval(FakeResponse(404))

    monkeypatch.setattr(mod.treq, "get", fake_get)
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("missing")))
    assert responder is None
    assert len(calls) == 1  # only the by-reference lookup ran
    # Neutral: the miss neither cleared nor incremented the real failure count.
    assert prov._read_breaker.state == _CLOSED
    assert prov._read_breaker._failures == 1


def test_fetch_content_404_is_neutral(monkeypatch):
    # Doc deleted between the by-reference lookup and the content GET (finding 7).
    prov = _make_provider()
    prov._read_breaker.on_failure()
    assert prov._read_breaker._failures == 1

    def fake_get(url, **kw):
        if "by-reference" in url:
            return _aval(FakeResponse(200, json_body={"id": "doc-9"}))
        return _aval(FakeResponse(404))

    monkeypatch.setattr(mod.treq, "get", fake_get)
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None
    assert prov._read_breaker._failures == 1  # neutral, not a failure


def test_fetch_malformed_meta_body_is_failure(monkeypatch):
    # A null / non-object JSON body must be a failure, not a swallowed
    # AttributeError → false miss (finding 12).
    prov = _make_provider()

    def fake_get(url, **kw):
        return _aval(FakeResponse(200, json_body=None))

    monkeypatch.setattr(mod.treq, "get", fake_get)
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None
    assert prov._read_breaker._failures == 1


def test_fetch_lookup_5xx_is_failure(monkeypatch):
    prov = _make_provider()

    def fake_get(url, **kw):
        return _aval(FakeResponse(500))

    monkeypatch.setattr(mod.treq, "get", fake_get)
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None
    assert prov._read_breaker._failures == 1


# --- circuit breaker -------------------------------------------------------


def _open_breaker(cb):
    """Drive a fresh breaker (fail_threshold=2) to OPEN."""
    cb.before_call()
    cb.on_failure()
    cb.before_call()
    cb.on_failure()  # threshold reached -> OPEN


def test_circuit_breaker_opens_and_recovers():
    t = {"now": 0.0}
    cb = _CircuitBreaker(fail_threshold=2, reset_timeout=10, clock=lambda: t["now"])

    _open_breaker(cb)
    assert cb.state == _OPEN
    with pytest.raises(_CircuitOpenError):
        cb.before_call()

    # after reset timeout -> HALF-OPEN admits a trial call
    t["now"] = 11.0
    cb.before_call()  # probe admitted
    assert cb.state == _HALF_OPEN
    cb.on_success()  # closes
    assert cb.state == _CLOSED
    cb.before_call()  # still closed


def test_half_open_admits_exactly_one_trial_under_concurrency():
    t = {"now": 0.0}
    cb = _CircuitBreaker(fail_threshold=2, reset_timeout=10, clock=lambda: t["now"])
    _open_breaker(cb)

    t["now"] = 11.0
    # First caller in the reset window is admitted as the single probe.
    cb.before_call()
    assert cb.state == _HALF_OPEN
    # Every other in-flight caller is still short-circuited (no thundering herd),
    # even though the reset timeout has elapsed.
    for _ in range(5):
        with pytest.raises(_CircuitOpenError):
            cb.before_call()


def test_half_open_failure_reopens(monkeypatch):
    # Finding 14: the half-open FAILURE path must re-open (previously untested).
    t = {"now": 0.0}
    cb = _CircuitBreaker(fail_threshold=2, reset_timeout=10, clock=lambda: t["now"])
    _open_breaker(cb)

    t["now"] = 11.0
    cb.before_call()  # probe admitted
    assert cb.state == _HALF_OPEN
    cb.on_failure()  # probe failed -> re-OPEN, timer restarted
    assert cb.state == _OPEN
    # Immediately after re-open the timer has been reset, so calls short-circuit.
    with pytest.raises(_CircuitOpenError):
        cb.before_call()
    # ...and only after ANOTHER full reset window is a new probe admitted.
    t["now"] = 22.0
    cb.before_call()
    assert cb.state == _HALF_OPEN


def test_half_open_neutral_recovers():
    t = {"now": 0.0}
    cb = _CircuitBreaker(fail_threshold=2, reset_timeout=10, clock=lambda: t["now"])
    _open_breaker(cb)
    t["now"] = 11.0
    cb.before_call()  # probe admitted
    cb.on_neutral()  # a clean miss proves the service is reachable -> CLOSED
    assert cb.state == _CLOSED
    cb.before_call()


def test_neutral_does_not_clear_failures_when_closed():
    cb = _CircuitBreaker(fail_threshold=5, reset_timeout=10)
    cb.before_call()
    cb.on_failure()
    cb.before_call()
    cb.on_failure()
    assert cb._failures == 2
    cb.on_neutral()  # a 404 miss must NOT zero real accumulated failures
    assert cb._failures == 2
    assert cb.state == _CLOSED


# --- streaming / responder (reactor code paths) ----------------------------


def test_consumer_sink_streams_with_backpressure_and_completes():
    from twisted.internet import defer

    consumer = FakeConsumer()
    finished = defer.Deferred()
    result = {}
    finished.addCallback(lambda n: result.__setitem__("written", n))

    sink = _ConsumerSink(consumer, finished)
    transport = FakeTransport()
    sink.makeConnection(transport)
    # Producer registered for real backpressure (streaming=True).
    assert consumer.producer is transport
    assert consumer.streaming is True

    sink.dataReceived(b"AB")
    sink.dataReceived(b"C")
    sink.connectionLost(None)  # clean close

    assert bytes(consumer.data) == b"ABC"
    assert consumer.unregistered is True
    assert result["written"] == 3
    assert finished.called


def test_consumer_sink_degrades_when_consumer_rejects_producer():
    from twisted.internet import defer

    consumer = FakeConsumer(reject_producer=True)
    finished = defer.Deferred()
    sink = _ConsumerSink(consumer, finished)
    sink.makeConnection(FakeTransport())  # must not raise
    sink.dataReceived(b"XY")
    sink.connectionLost(None)
    assert bytes(consumer.data) == b"XY"


def test_consumer_sink_reports_mid_stream_failure():
    from twisted.internet import defer
    from twisted.python.failure import Failure

    consumer = FakeConsumer()
    finished = defer.Deferred()
    errors = {}
    finished.addErrback(lambda f: errors.__setitem__("err", f) or None)

    sink = _ConsumerSink(consumer, finished)
    sink.makeConnection(FakeTransport())
    sink.dataReceived(b"AB")
    sink.connectionLost(Failure(RuntimeError("boom")))  # abrupt drop
    assert "err" in errors


def test_responder_write_to_consumer_streams_and_records_success():
    outcomes = []
    response = FakeResponse(200, body=b"CONTENT")
    responder = _FileServiceResponder(
        response,
        on_success=lambda: outcomes.append("success"),
        on_failure=lambda: outcomes.append("failure"),
        on_neutral=lambda: outcomes.append("neutral"),
    )
    consumer = FakeConsumer()

    with responder:
        responder.write_to_consumer(consumer)
        sink = response.delivered_to
        sink.dataReceived(b"CONTENT")
        sink.connectionLost(None)  # clean completion

    assert bytes(consumer.data) == b"CONTENT"
    # Success is recorded on stream completion (not before) — and exactly once.
    assert outcomes == ["success"]


def test_responder_stream_failure_records_failure():
    from twisted.python.failure import Failure

    outcomes = []
    response = FakeResponse(200, body=b"CONTENT")
    responder = _FileServiceResponder(
        response,
        on_success=lambda: outcomes.append("success"),
        on_failure=lambda: outcomes.append("failure"),
        on_neutral=lambda: outcomes.append("neutral"),
    )
    consumer = FakeConsumer()

    with responder:
        responder.write_to_consumer(consumer)
        sink = response.delivered_to
        sink.dataReceived(b"CON")
        sink.connectionLost(Failure(RuntimeError("mid-stream drop")))

    assert outcomes == ["failure"]


def test_responder_exit_without_stream_aborts_and_is_neutral():
    # Finding 2: entering then exiting WITHOUT streaming must release/abort the
    # unbuffered connection and free the breaker trial as neutral.
    outcomes = []
    response = FakeResponse(200, body=b"CONTENT")
    responder = _FileServiceResponder(
        response,
        on_success=lambda: outcomes.append("success"),
        on_failure=lambda: outcomes.append("failure"),
        on_neutral=lambda: outcomes.append("neutral"),
    )

    with responder:
        pass  # Synapse never calls write_to_consumer (client disconnect)

    # The unconsumed body was delivered to the aborting protocol, which stopped
    # the transport (connection released, not leaked).
    assert isinstance(response.delivered_to, _DrainAndAbort)
    assert response.transport.stopped is True
    assert outcomes == ["neutral"]


def test_responder_fully_streamed_then_exit_does_not_double_abort():
    outcomes = []
    response = FakeResponse(200, body=b"CONTENT")
    responder = _FileServiceResponder(
        response, on_success=lambda: outcomes.append("success")
    )
    consumer = FakeConsumer()

    with responder:
        responder.write_to_consumer(consumer)
        sink = response.delivered_to
        sink.dataReceived(b"CONTENT")
        sink.connectionLost(None)

    # __exit__ after a full stream must NOT abort again (streamed path).
    assert not isinstance(response.delivered_to, _DrainAndAbort)
    assert outcomes == ["success"]

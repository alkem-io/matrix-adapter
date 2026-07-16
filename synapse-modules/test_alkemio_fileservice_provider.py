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

import types

import pytest

from twisted.internet import defer
from twisted.internet.defer import TimeoutError as _TimeoutError
from twisted.internet.task import Clock
from twisted.python.failure import Failure

import alkemio_fileservice_provider as mod
from alkemio_fileservice_provider import (
    FileServiceStorageProvider,
    _ConsumerSink,
    _DrainAndAbort,
    _FileServiceResponder,
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
    def __init__(self, code, json_body=None, body=b"", content_error=None):
        self.code = code
        self._json = json_body
        self._body = body
        self._content_error = content_error
        self.delivered_to = None
        self.transport = None

    def content(self):
        if self._content_error is not None:
            async def _raise():
                raise self._content_error

            return _raise()
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


async def _araise(exc):
    """An awaitable that raises — stands in for a treq request timeout/failure."""
    raise exc


def _result_of(d):
    """Extract the result of a Deferred that has (or must have) fired synchronously."""
    box = {}
    d.addCallbacks(
        lambda v: box.__setitem__("ok", v),
        lambda f: box.__setitem__("err", f),
    )
    if "err" in box:
        box["err"].raiseException()
    if "ok" in box:
        return box["ok"]
    raise AssertionError("coroutine did not complete synchronously (still pending)")


def _run(coro):
    """Drive a provider coroutine to synchronous completion (twisted driver).

    The provider bounds three awaits with a reactor-level per-op timeout
    (`_with_timeout` -> Deferred.addTimeout), so we drive coroutines with
    twisted's own adapter and a `Clock` reactor rather than asyncio (asyncio
    cannot await the twisted Deferreds `_with_timeout` produces). Stall tests
    instead call `defer.ensureDeferred(...)`, advance `prov.reactor`, then
    `_result_of(...)`.
    """
    return _result_of(defer.ensureDeferred(coro))


def _make_provider(**config_overrides):
    base = {
        "file_service_url": "http://file-service:4003/",
        "matrix_media_bucket_id": "00000000-0000-0000-0000-0000000000ff",
    }
    base.update(config_overrides)
    cfg = FileServiceStorageProvider.parse_config(base)
    # A twisted Clock stands in for the reactor: it provides callLater (needed by
    # `_with_timeout`'s addTimeout) and lets stall tests advance virtual time.
    hs = types.SimpleNamespace(
        get_reactor=Clock,
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


def test_parse_config_defaults_and_no_cb_keys():
    cfg = FileServiceStorageProvider.parse_config(
        {"file_service_url": "http://fs:4003", "matrix_media_bucket_id": "b"}
    )
    assert cfg["timeout_s"] == mod.DEFAULT_TIMEOUT_S
    assert cfg["store_timeout_s"] == mod.DEFAULT_STORE_TIMEOUT_S
    # The removed circuit-breaker knobs are gone from the parsed config.
    assert "cb_fail_threshold" not in cfg
    assert "cb_reset_timeout_s" not in cfg


def test_parse_config_present_but_null_falls_back_to_default():
    # A YAML `timeout_s:` with no value parses to None; it must fall back to the
    # default, NOT blow up in float(None).
    cfg = FileServiceStorageProvider.parse_config(
        {
            "file_service_url": "http://fs:4003",
            "matrix_media_bucket_id": "b",
            "timeout_s": None,
            "store_timeout_s": None,
        }
    )
    assert cfg["timeout_s"] == mod.DEFAULT_TIMEOUT_S
    assert cfg["store_timeout_s"] == mod.DEFAULT_STORE_TIMEOUT_S


@pytest.mark.parametrize(
    "overrides",
    [
        {"timeout_s": 0},
        {"store_timeout_s": -5},
        {"timeout_s": "not-a-number"},
        # NaN sneaks past `<= 0` (all NaN comparisons are False) and +inf passes
        # `> 0` — both must be rejected as non-finite.
        {"timeout_s": float("nan")},
        {"timeout_s": float("inf")},
        {"store_timeout_s": float("-inf")},
        # bool is an int subclass — YAML `true`/`false` must NOT pass as 1/0.
        {"timeout_s": True},
        {"store_timeout_s": False},
        # An astronomically large int makes float() overflow; must surface as a
        # clean ValueError at boot, not a raw traceback.
        {"timeout_s": 10 ** 400},
        {"store_timeout_s": 10 ** 400},
    ],
)
def test_parse_config_rejects_bad_tuning(overrides):
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
        captured["timeout"] = kw.get("timeout")
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
    # Per-request timeout applied.
    assert captured["timeout"] == prov.store_timeout_s
    # The body must be the STREAMED file handle, not a fully-buffered bytes blob.
    body = captured["files"]["file"][1]
    assert body is fake_file
    assert not isinstance(body, (bytes, bytearray))
    assert hasattr(body, "read")
    # And the handle is closed once the post completes.
    assert fake_file.closed is True


def test_store_raises_and_closes_handle_on_http_error(monkeypatch):
    prov = _make_provider()
    fake_file = FakeFile(b"x")
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(FakeResponse(500)))
    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)
    with pytest.raises(RuntimeError):
        _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    # finally: closes the handle even on the error path.
    assert fake_file.closed is True


@pytest.mark.parametrize("code", [200, 202, 204, 301, 302])
def test_store_rejects_non_201_success(monkeypatch, code):
    # A 2xx-non-201 or a 3xx is NOT a confirmed durable store.
    prov = _make_provider()
    fake_file = FakeFile(b"x")
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(FakeResponse(code)))
    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)
    with pytest.raises(RuntimeError):
        _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    assert fake_file.closed is True


def test_store_201_durable_despite_drain_error(monkeypatch):
    # Once 201 is confirmed the store is durable; a failure while draining the
    # response body must NOT turn a successful store into a failure.
    prov = _make_provider()
    fake_file = FakeFile(b"x")
    resp = FakeResponse(201, content_error=RuntimeError("connection reset on drain"))
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(resp))
    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)

    # Must NOT raise.
    _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    assert fake_file.closed is True


def test_store_post_timeout_raises_and_closes_handle(monkeypatch):
    # A per-request store timeout surfaces as an exception; store_synchronous
    # then reports the upload failure loudly. The handle is still closed.
    prov = _make_provider()
    fake_file = FakeFile(b"x")
    monkeypatch.setattr(
        mod.treq, "post", lambda *a, **k: _araise(_TimeoutError("store timed out"))
    )
    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)
    with pytest.raises(_TimeoutError):
        _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    assert fake_file.closed is True


# --- fetch -----------------------------------------------------------------


def test_fetch_global_lookup_then_streams(monkeypatch):
    prov = _make_provider()
    calls = []
    timeouts = []

    def fake_get(url, **kw):
        calls.append(url)
        timeouts.append(kw.get("timeout"))
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
    # Each request carries its own per-request timeout.
    assert timeouts == [prov.timeout_s, prov.timeout_s]
    assert responder is not None
    assert isinstance(responder, _FileServiceResponder)


def test_fetch_returns_none_on_by_reference_404(monkeypatch):
    prov = _make_provider()
    calls = []

    def fake_get(url, **kw):
        calls.append(url)
        return _aval(FakeResponse(404))

    monkeypatch.setattr(mod.treq, "get", fake_get)
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("missing")))
    assert responder is None
    assert len(calls) == 1  # only the by-reference lookup ran


def test_fetch_returns_none_on_content_404(monkeypatch):
    # Doc deleted between the by-reference lookup and the content GET.
    prov = _make_provider()

    def fake_get(url, **kw):
        if "by-reference" in url:
            return _aval(FakeResponse(200, json_body={"id": "doc-9"}))
        return _aval(FakeResponse(404))

    monkeypatch.setattr(mod.treq, "get", fake_get)
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None


def test_fetch_returns_none_on_lookup_5xx(monkeypatch):
    prov = _make_provider()
    monkeypatch.setattr(mod.treq, "get", lambda url, **kw: _aval(FakeResponse(500)))
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))
    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None


def test_fetch_returns_none_on_malformed_meta_body(monkeypatch):
    # A null / non-object JSON body must be treated as a miss, not a swallowed
    # AttributeError.
    prov = _make_provider()
    monkeypatch.setattr(
        mod.treq, "get", lambda url, **kw: _aval(FakeResponse(200, json_body=None))
    )
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))
    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None


def test_fetch_returns_none_on_transport_error(monkeypatch):
    prov = _make_provider()
    monkeypatch.setattr(
        mod.treq, "get", lambda url, **kw: _araise(ConnectionError("refused"))
    )
    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None


def test_fetch_returns_none_on_request_timeout(monkeypatch):
    # A per-request fetch timeout degrades to a cache miss (None), never a hang.
    prov = _make_provider()
    monkeypatch.setattr(
        mod.treq, "get", lambda url, **kw: _araise(_TimeoutError("lookup timed out"))
    )
    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None


# --- stalled-await per-op timeouts (treq's timeout only covers headers) ----


def test_fetch_stalled_body_read_times_out_returns_none(monkeypatch):
    # The by-reference GET returns 200 headers, but the JSON BODY read never
    # resolves. treq's request timeout only covered the headers, so `_with_timeout`
    # must fire and fetch must return None (a cache miss) rather than hang.
    prov = _make_provider()
    monkeypatch.setattr(
        mod.treq,
        "get",
        lambda url, **kw: _aval(FakeResponse(200, json_body={"id": "doc-9"})),
    )
    # The body read never completes.
    monkeypatch.setattr(mod.treq, "json_content", lambda r: defer.Deferred())

    d = defer.ensureDeferred(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    # Still pending: the body read is stalled.
    prov.reactor.advance(prov.timeout_s + 1)  # trip the per-op deadline
    assert _result_of(d) is None


def test_store_hung_open_times_out_raises(monkeypatch):
    # A blocking open() on a wedged media-store mount never resolves. The
    # `store_timeout_s` per-op deadline must fire so store_file RAISES (upload
    # fails loudly under store_synchronous) rather than hanging. Nothing was
    # opened, so no handle is leaked in the coroutine.
    prov = _make_provider()
    # The off-reactor open never completes.
    monkeypatch.setattr(
        mod, "defer_to_thread", lambda reactor, fn, *a: defer.Deferred()
    )
    posted = {"n": 0}
    monkeypatch.setattr(
        mod.treq,
        "post",
        lambda *a, **k: posted.__setitem__("n", posted["n"] + 1)
        or _aval(FakeResponse(201)),
    )

    d = defer.ensureDeferred(prov.store_file("local_content/x", FakeFileInfo("m")))
    prov.reactor.advance(prov.store_timeout_s + 1)  # trip the per-op deadline
    with pytest.raises(_TimeoutError):
        _result_of(d)
    assert posted["n"] == 0  # never reached the post (open hung)


def test_fetch_stalled_404_drain_still_returns_none(monkeypatch):
    # Even if the small 404 body read stalls, the miss path still returns None:
    # _drain_quietly bounds the drain and swallows the resulting timeout.
    prov = _make_provider()
    stalling = FakeResponse(404)
    # content() never resolves.
    stalling.content = lambda: defer.Deferred()
    monkeypatch.setattr(mod.treq, "get", lambda url, **kw: _aval(stalling))
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))

    d = defer.ensureDeferred(prov.fetch("local_content/x", FakeFileInfo("missing")))
    prov.reactor.advance(prov.timeout_s + 1)  # trip the drain deadline
    assert _result_of(d) is None


# --- streaming / responder (reactor code paths) ----------------------------


def test_consumer_sink_streams_with_backpressure_and_completes():
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
    consumer = FakeConsumer(reject_producer=True)
    finished = defer.Deferred()
    sink = _ConsumerSink(consumer, finished)
    sink.makeConnection(FakeTransport())  # must not raise
    sink.dataReceived(b"XY")
    sink.connectionLost(None)
    assert bytes(consumer.data) == b"XY"


def test_consumer_sink_reports_mid_stream_failure():
    consumer = FakeConsumer()
    finished = defer.Deferred()
    errors = {}
    finished.addErrback(lambda f: errors.__setitem__("err", f) or None)

    sink = _ConsumerSink(consumer, finished)
    sink.makeConnection(FakeTransport())
    sink.dataReceived(b"AB")
    sink.connectionLost(Failure(RuntimeError("boom")))  # abrupt drop
    assert "err" in errors


def test_responder_write_to_consumer_streams_bytes_to_consumer():
    response = FakeResponse(200, body=b"CONTENT")
    responder = _FileServiceResponder(response)
    consumer = FakeConsumer()

    result = {}
    with responder:
        d = responder.write_to_consumer(consumer)
        d.addCallback(lambda n: result.__setitem__("written", n))
        sink = response.delivered_to
        sink.dataReceived(b"CONTENT")
        sink.connectionLost(None)  # clean completion

    assert bytes(consumer.data) == b"CONTENT"
    assert result["written"] == len(b"CONTENT")


def test_responder_sync_deliverbody_raise_errbacks_and_aborts():
    # A SYNCHRONOUS deliverBody raise must (a) errback `finished` so the caller
    # does not hang, and (b) leave `_streamed` False so `__exit__` aborts/releases
    # the unbuffered connection. Driven INSIDE `with` so __exit__ runs.
    response = FakeResponse(200, body=b"CONTENT")
    calls = {"n": 0}
    orig_deliver = response.deliverBody

    def boom_first_then_abort(protocol):
        calls["n"] += 1
        if calls["n"] == 1:
            raise RuntimeError("deliverBody exploded synchronously")
        # Second call is the __exit__ abort path — deliver normally so the
        # _DrainAndAbort protocol runs and stops the transport.
        orig_deliver(protocol)

    response.deliverBody = boom_first_then_abort
    responder = _FileServiceResponder(response)
    consumer = FakeConsumer()

    errors = {}
    with responder:
        d = responder.write_to_consumer(consumer)
        d.addErrback(lambda f: errors.__setitem__("err", f) or None)

    assert "err" in errors  # errback fired synchronously, caller does not hang
    assert isinstance(response.delivered_to, _DrainAndAbort)
    assert response.transport.stopped is True


def test_responder_exit_without_stream_aborts_connection():
    # Entering then exiting WITHOUT streaming must release/abort the unbuffered
    # connection (pool safety).
    response = FakeResponse(200, body=b"CONTENT")
    responder = _FileServiceResponder(response)

    with responder:
        pass  # Synapse never calls write_to_consumer (client disconnect)

    assert isinstance(response.delivered_to, _DrainAndAbort)
    assert response.transport.stopped is True


def test_responder_fully_streamed_then_exit_does_not_double_abort():
    response = FakeResponse(200, body=b"CONTENT")
    responder = _FileServiceResponder(response)
    consumer = FakeConsumer()

    with responder:
        responder.write_to_consumer(consumer)
        sink = response.delivered_to
        sink.dataReceived(b"CONTENT")
        sink.connectionLost(None)

    # __exit__ after a full stream must NOT abort again (streamed path).
    assert not isinstance(response.delivered_to, _DrainAndAbort)

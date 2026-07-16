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

import json
import types

import pytest

from twisted.internet import defer
from twisted.internet.defer import TimeoutError as _TimeoutError
from twisted.internet.task import Clock
from twisted.python.failure import Failure
from twisted.web.client import ResponseDone

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
    """
    An UNBUFFERED response stand-in.

    - abort path (`_abort_connection` / a stalled drain-or-read): deliverBody ->
      makeConnection -> transport.stopProducing() sets `transport.stopped`.
    - drain/keep-alive path: `auto_body` (incl. b"") makes deliverBody feed the
      body then close cleanly -> the drainer completes -> connection reused
      (transport.stopped stays False).
    - `stall_body=True`: deliverBody connects but never delivers (stalled body ->
      the caller times out and aborts).
    """

    def __init__(self, code, auto_body=None, stall_body=False):
        self.code = code
        self._auto_body = auto_body
        self._stall_body = stall_body
        self.delivered_to = None
        self.transport = None

    def deliverBody(self, protocol):
        self.delivered_to = protocol
        self.transport = FakeTransport()
        protocol.makeConnection(self.transport)
        if self._stall_body:
            return  # never delivers; caller must abort to release it
        if self._auto_body is not None:
            if self._auto_body:
                protocol.dataReceived(self._auto_body)
            protocol.connectionLost(Failure(ResponseDone()))  # clean close -> reused


def _meta(obj):
    """A by-reference meta response whose body auto-delivers as JSON."""
    return FakeResponse(200, auto_body=json.dumps(obj).encode())


def _drainable(code):
    """A non-streamed reply whose (empty) body drains cleanly -> connection reused
    (transport NOT aborted)."""
    return FakeResponse(code, auto_body=b"")


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

    The provider bounds body reads with a reactor-level per-op timeout
    (`_with_timeout` -> Deferred.addTimeout), so we drive coroutines with
    twisted's own adapter and a `Clock` reactor. Stall tests instead call
    `defer.ensureDeferred(...)`, advance `prov.reactor`, then `_result_of(...)`.
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
    # make_deferred_yieldable passes the awaitable through. defer_to_thread runs
    # the function synchronously and returns a (fired) Deferred — the provider now
    # calls addTimeout on it, so it must be a real Deferred, not a coroutine.
    monkeypatch.setattr(mod, "make_deferred_yieldable", lambda d: d)
    monkeypatch.setattr(
        mod, "defer_to_thread", lambda reactor, fn, *a: defer.execute(fn, *a)
    )


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
    store_resp = _drainable(201)

    def fake_post(url, files=None, data=None, **kw):
        captured["url"] = url
        captured["files"] = files
        captured["data"] = data
        captured["timeout"] = kw.get("timeout")
        captured["unbuffered"] = kw.get("unbuffered")
        return _aval(store_resp)

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
    # Per-request timeout + unbuffered (so the reply can be drained/released).
    assert captured["timeout"] == prov.store_timeout_s
    assert captured["unbuffered"] is True
    # The body must be the STREAMED file handle, not a fully-buffered bytes blob.
    body = captured["files"]["file"][1]
    assert body is fake_file
    assert not isinstance(body, (bytes, bytearray))
    assert hasattr(body, "read")
    # Handle closed; and the 201 reply body was DRAINED (keep-alive) not aborted.
    assert fake_file.closed is True
    assert store_resp.transport.stopped is False


def test_store_raises_and_closes_handle_on_http_error(monkeypatch):
    prov = _make_provider()
    fake_file = FakeFile(b"x")
    err_resp = _drainable(500)
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(err_resp))
    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)
    with pytest.raises(RuntimeError):
        _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    # finally: closes the handle even on the error path; the small error body is
    # drained (keep-alive), not aborted.
    assert fake_file.closed is True
    assert err_resp.transport.stopped is False


@pytest.mark.parametrize("code", [200, 202, 204, 301, 302])
def test_store_rejects_non_201_success(monkeypatch, code):
    # A 2xx-non-201 or a 3xx is NOT a confirmed durable store.
    prov = _make_provider()
    fake_file = FakeFile(b"x")
    resp = _drainable(code)
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(resp))
    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)
    with pytest.raises(RuntimeError):
        _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    assert fake_file.closed is True
    assert resp.transport.stopped is False  # drained (reused), not aborted


def test_store_201_durable_despite_release_failure(monkeypatch):
    # Once 201 is confirmed the store is durable; a failure while draining/
    # releasing the reply body must NOT turn a successful store into a failure.
    prov = _make_provider()
    fake_file = FakeFile(b"x")
    resp = FakeResponse(201)

    def boom_deliver(protocol):
        raise RuntimeError("connection reset while draining the 201 reply")

    resp.deliverBody = boom_deliver
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(resp))
    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)

    # Must NOT raise — the durable store stands.
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


def test_store_late_open_closes_late_handle(monkeypatch):
    # The realistic case: open() completes AFTER the store already timed out
    # (mount recovered). The guarded open must CLOSE the late-returned handle
    # itself so the FD is not orphaned.
    prov = _make_provider()
    fake_file = FakeFile(b"x")
    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)

    captured = {}

    def fake_dtt(reactor, fn, *a):
        captured["fn"] = fn  # the guarded-open closure
        return defer.Deferred()  # never fires on its own

    monkeypatch.setattr(mod, "defer_to_thread", fake_dtt)
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(FakeResponse(201)))

    d = defer.ensureDeferred(prov.store_file("local_content/x", FakeFileInfo("m")))
    prov.reactor.advance(prov.store_timeout_s + 1)  # timeout -> store raises
    with pytest.raises(_TimeoutError):
        _result_of(d)
    assert fake_file.closed is False  # nothing opened yet

    # Now the "thread" completes late (mount recovered): run the guarded open.
    with pytest.raises(mod._OpenAfterTimeout):
        captured["fn"]()
    assert fake_file.closed is True  # late handle self-closed, not leaked


def test_store_open_not_routed_through_with_timeout(monkeypatch):
    # Structural logcontext guard: the cache-file open must be awaited via
    # addTimeout DIRECTLY on the already-yieldable defer_to_thread Deferred, never
    # through `_with_timeout` (which adds a second make_deferred_yieldable that
    # would resume the upload under the sentinel logcontext). The autouse fixture
    # stubs make_deferred_yieldable to identity, so the wrap is invisible at
    # runtime — spying `_with_timeout` and its timeout arg is the structural proxy.
    # (`_with_timeout` IS legitimately used for the reply DRAIN, with `timeout_s`;
    # the open, if wrapped, would show up with `store_timeout_s`.)
    prov = _make_provider()
    assert prov.store_timeout_s != prov.timeout_s  # so the two are distinguishable
    calls = []
    orig = mod._with_timeout

    async def spy(reactor, timeout_s, d):
        calls.append(timeout_s)
        return await orig(reactor, timeout_s, d)

    monkeypatch.setattr(mod, "_with_timeout", spy)
    monkeypatch.setattr(mod, "_open_stream", lambda p: FakeFile(b"x"))
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(_drainable(201)))

    _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    # The open is NOT wrapped: `store_timeout_s` never reaches _with_timeout.
    assert prov.store_timeout_s not in calls
    # ...and the guard is not vacuous — the reply DRAIN does use _with_timeout.
    assert prov.timeout_s in calls


# --- fetch -----------------------------------------------------------------


def test_fetch_global_lookup_then_streams(monkeypatch):
    prov = _make_provider()
    calls = []
    timeouts = []
    unbuffered = []

    def fake_get(url, **kw):
        calls.append(url)
        timeouts.append(kw.get("timeout"))
        unbuffered.append(kw.get("unbuffered"))
        if "by-reference" in url:
            return _aval(_meta({"id": "doc-9"}))
        return _aval(FakeResponse(200))

    monkeypatch.setattr(mod.treq, "get", fake_get)

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))

    # global lookup: NO bucketId query param
    assert (
        calls[0]
        == "http://file-service:4003/internal/file/by-reference?ref=MEDIAID"
    )
    assert "bucketId" not in calls[0]
    assert calls[1] == "http://file-service:4003/internal/file/doc-9/content"
    # Each request carries its own per-request timeout, and BOTH are unbuffered.
    assert timeouts == [prov.timeout_s, prov.timeout_s]
    assert unbuffered == [True, True]
    assert responder is not None
    assert isinstance(responder, _FileServiceResponder)


def test_fetch_by_reference_bom_body_parsed(monkeypatch):
    # [0] The body is parsed with json.loads(BYTES) — RFC 8259 JSON is UTF-8/16/32
    # and json's detect_encoding handles a UTF-8 BOM. A BOM-prefixed body must
    # parse (the old charset-decode path raised "Unexpected UTF-8 BOM" and lost
    # the doc as a false miss).
    prov = _make_provider()
    bom_body = b"\xef\xbb\xbf" + json.dumps({"id": "doc-9"}).encode()
    meta = FakeResponse(200, auto_body=bom_body)
    calls = []

    def fake_get(url, **kw):
        calls.append(url)
        if "by-reference" in url:
            return _aval(meta)
        return _aval(FakeResponse(200))

    monkeypatch.setattr(mod.treq, "get", fake_get)
    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is not None
    # Parsed the BOM-prefixed id and proceeded to the content GET.
    assert calls[1] == "http://file-service:4003/internal/file/doc-9/content"


def test_fetch_by_reference_body_over_cap_aborts(monkeypatch):
    # [4] A fast oversized by-reference body must be aborted (not buffered) and
    # the fetch degrades to None.
    prov = _make_provider()
    big = FakeResponse(200, auto_body=b"x" * (mod._MAX_META_BYTES + 1))
    monkeypatch.setattr(mod.treq, "get", lambda url, **kw: _aval(big))
    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None
    assert big.transport.stopped is True  # oversized body aborted


def test_fetch_returns_none_on_by_reference_404_and_drains(monkeypatch):
    prov = _make_provider()
    calls = []
    miss = _drainable(404)

    def fake_get(url, **kw):
        calls.append(url)
        return _aval(miss)

    monkeypatch.setattr(mod.treq, "get", fake_get)

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("missing")))
    assert responder is None
    assert len(calls) == 1  # only the by-reference lookup ran
    assert miss.transport.stopped is False  # small body drained (keep-alive)


def test_fetch_returns_none_on_content_404_and_drains(monkeypatch):
    # Doc deleted between the by-reference lookup and the content GET.
    prov = _make_provider()
    content_miss = _drainable(404)

    def fake_get(url, **kw):
        if "by-reference" in url:
            return _aval(_meta({"id": "doc-9"}))
        return _aval(content_miss)

    monkeypatch.setattr(mod.treq, "get", fake_get)

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None
    assert content_miss.transport.stopped is False  # drained (keep-alive)


def test_fetch_returns_none_on_lookup_5xx_and_drains(monkeypatch):
    prov = _make_provider()
    err = _drainable(500)
    monkeypatch.setattr(mod.treq, "get", lambda url, **kw: _aval(err))
    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None
    assert err.transport.stopped is False  # drained (keep-alive)


def test_fetch_returns_none_on_malformed_meta_body(monkeypatch):
    # A null / non-object JSON body must be treated as a miss, not a swallowed
    # AttributeError. (The body was fully read, so the connection is already done.)
    prov = _make_provider()
    monkeypatch.setattr(mod.treq, "get", lambda url, **kw: _aval(_meta(None)))
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


# --- stalled body read: per-op timeout + release ---------------------------


def test_fetch_stalled_body_read_times_out_and_releases_conn(monkeypatch):
    # The by-reference GET returns 200 headers, but the JSON BODY read never
    # resolves. treq's request timeout only covered the headers, so the per-op
    # deadline must fire, fetch must return None, AND the stalled connection must
    # be released (aborted) — not leaked.
    prov = _make_provider()
    stalling_meta = FakeResponse(200, stall_body=True)  # deliverBody never delivers
    monkeypatch.setattr(mod.treq, "get", lambda url, **kw: _aval(stalling_meta))

    d = defer.ensureDeferred(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    # Still pending: the body read is stalled.
    prov.reactor.advance(prov.timeout_s + 1)  # trip the per-op deadline
    assert _result_of(d) is None
    # The stalled body connection was aborted (transport.stopProducing called).
    assert stalling_meta.transport.stopped is True


def test_fetch_stalled_reply_drain_times_out_and_aborts(monkeypatch):
    # A NON-streamed reply (a 404 miss) whose body STALLS: the keep-alive drain is
    # bounded, so on timeout it aborts (tears the connection down) instead of
    # hanging — and fetch still returns None. This is the drained-vs-aborted
    # counterpart to the fast-drain (reused) misses above.
    prov = _make_provider()
    stalling_miss = FakeResponse(404, stall_body=True)
    monkeypatch.setattr(mod.treq, "get", lambda url, **kw: _aval(stalling_miss))

    d = defer.ensureDeferred(prov.fetch("local_content/x", FakeFileInfo("missing")))
    prov.reactor.advance(prov.timeout_s + 1)  # trip the drain deadline
    assert _result_of(d) is None
    assert stalling_miss.transport.stopped is True  # drain gave up -> aborted


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
    response = FakeResponse(200)
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
    response = FakeResponse(200)
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
    response = FakeResponse(200)
    responder = _FileServiceResponder(response)

    with responder:
        pass  # Synapse never calls write_to_consumer (client disconnect)

    assert isinstance(response.delivered_to, _DrainAndAbort)
    assert response.transport.stopped is True


def test_responder_fully_streamed_then_exit_does_not_double_abort():
    response = FakeResponse(200)
    responder = _FileServiceResponder(response)
    consumer = FakeConsumer()

    with responder:
        responder.write_to_consumer(consumer)
        sink = response.delivered_to
        sink.dataReceived(b"CONTENT")
        sink.connectionLost(None)

    # __exit__ after a full stream must NOT abort again (streamed path).
    assert not isinstance(response.delivered_to, _DrainAndAbort)

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

from twisted.internet import defer as _tdefer
from twisted.internet.defer import TimeoutError as _TimeoutError
from twisted.internet.task import Clock
from twisted.python.failure import Failure

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

    The provider now wraps its guarded body in twisted's reactor-level timeout
    (`_bounded`), so we drive coroutines with twisted's own coroutine adapter (and
    a `Clock` reactor) rather than asyncio — otherwise the twisted Deferreds the
    provider awaits could not be awaited. Timeout tests instead call
    `_tdefer.ensureDeferred(...)`, advance `prov.reactor`, then `_result_of(...)`.
    """
    return _result_of(_tdefer.ensureDeferred(coro))


def _make_provider(**config_overrides):
    base = {
        "file_service_url": "http://file-service:4003/",
        "matrix_media_bucket_id": "00000000-0000-0000-0000-0000000000ff",
    }
    base.update(config_overrides)
    cfg = FileServiceStorageProvider.parse_config(base)
    # A twisted Clock stands in for the reactor: it provides callLater (needed by
    # `_bounded`'s addTimeout) and lets timeout tests advance virtual time.
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
        # finding 5: NaN sneaks past `<= 0` (all NaN comparisons are False) and
        # +inf passes `> 0` — both must be rejected as non-finite.
        {"timeout_s": float("nan")},
        {"timeout_s": float("inf")},
        {"store_timeout_s": float("-inf")},
        # finding 6: bool is an int subclass — YAML `true`/`false` must NOT pass
        # as 1/0.
        {"cb_fail_threshold": True},
        {"timeout_s": False},
        # finding 0: an astronomically large int makes int() succeed but
        # math.isfinite(value) raise OverflowError — it must surface as a clean
        # ValueError at boot, not a raw traceback.
        {"cb_fail_threshold": 10 ** 400},
        {"timeout_s": 10 ** 400},
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


def test_store_201_durable_despite_drain_error(monkeypatch):
    # Finding 4: once 201 is confirmed the store is durable; a failure while
    # draining the response body must NOT turn a successful store into a failure.
    prov = _make_provider()
    fake_file = FakeFile(b"x")
    resp = FakeResponse(201, content_error=RuntimeError("connection reset on drain"))
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(resp))
    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)

    # Must NOT raise.
    _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    assert prov._write_breaker.state == _CLOSED  # recorded as success
    assert fake_file.closed is True


def test_store_open_failure_resolves_breaker_no_wedge(monkeypatch):
    # Finding 0: a failure BEFORE the post (here the file open) must still resolve
    # the write breaker — a HALF_OPEN probe must never wedge un-resolved.
    prov = _make_provider(cb_fail_threshold=1)

    def boom(_path):
        raise OSError("cache file vanished")

    posted = {"called": False}
    monkeypatch.setattr(mod, "_open_stream", boom)
    monkeypatch.setattr(
        mod.treq, "post", lambda *a, **k: posted.__setitem__("called", True)
    )
    with pytest.raises(OSError):
        _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    assert posted["called"] is False  # never reached the post
    assert prov._write_breaker.state == _OPEN  # failure recorded exactly once


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

    # Pre-load a real failure. Response-received (`on_response`) resolves the
    # request but in CLOSED it deliberately does NOT reset the failure count —
    # otherwise a truncating backend (which always returns a 200 first) could
    # never accumulate failures and trip the breaker (finding 4). So the pre-
    # existing failure survives; only a HALF_OPEN probe response resets.
    prov._read_breaker.on_failure()
    assert prov._read_breaker._failures == 1

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))

    # global lookup: NO bucketId query param
    assert (
        calls[0]
        == "http://file-service:4003/internal/file/by-reference?ref=MEDIAID"
    )
    assert "bucketId" not in calls[0]
    assert calls[1] == "http://file-service:4003/internal/file/doc-9/content"
    assert responder is not None
    # Response-received resolved the request without opening; CLOSED failure count
    # is left intact (not zeroed) so post-response truncations can still shed.
    assert prov._read_breaker.state == _CLOSED
    assert prov._read_breaker._failures == 1


def test_fetch_success_resolves_half_open_probe_at_response(monkeypatch):
    # A HALF_OPEN read probe resolves the instant the 200 arrives — it does NOT
    # stay HALF_OPEN for the whole download (finding 1). We drive the read breaker
    # OPEN, let the reset window elapse, then a successful fetch (response
    # received) must leave it CLOSED even though no body was streamed.
    t = {"now": 0.0}
    prov = _make_provider(cb_fail_threshold=1, cb_reset_timeout_s=10)
    prov._read_breaker._clock = lambda: t["now"]
    prov._read_breaker.on_failure()  # threshold=1 -> OPEN
    assert prov._read_breaker.state == _OPEN

    def fake_get(url, **kw):
        if "by-reference" in url:
            return _aval(FakeResponse(200, json_body={"id": "doc-9"}))
        return _aval(FakeResponse(200, body=b"CONTENT"))

    monkeypatch.setattr(mod.treq, "get", fake_get)
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))

    t["now"] = 11.0  # reset window elapsed -> next call is the single probe
    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is not None
    assert prov._read_breaker.state == _CLOSED  # probe resolved at response


def test_fetch_404_drain_error_stays_neutral(monkeypatch):
    # Finding 3: a 404 records neutral, and a failure while draining its small
    # body must NOT reach the outer except and invert neutral into a failure.
    prov = _make_provider()
    prov._read_breaker.on_failure()
    assert prov._read_breaker._failures == 1

    def fake_get(url, **kw):
        return _aval(
            FakeResponse(404, content_error=RuntimeError("drain blew up"))
        )

    monkeypatch.setattr(mod.treq, "get", fake_get)
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("missing")))
    assert responder is None
    # Neutral survived the drain error: still 1 failure, still CLOSED (not 2).
    assert prov._read_breaker.state == _CLOSED
    assert prov._read_breaker._failures == 1


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


# --- bounded-await timeouts (no wedge) -------------------------------------


def test_fetch_stalled_body_read_times_out_and_resolves_breaker(monkeypatch):
    # Findings 1/5: treq's request timeout only guards up to headers. A backend
    # that returns by-reference 200 headers then STALLS the JSON body read would
    # hang the await forever — the `timeout_s` reactor deadline must fire, the
    # fetch resolves as a failure (breaker resolved, no wedge), and returns None.
    prov = _make_provider()
    monkeypatch.setattr(
        mod.treq,
        "get",
        lambda url, **kw: _aval(FakeResponse(200, json_body={"id": "doc-9"})),
    )
    # The by-reference JSON body read never completes.
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _tdefer.Deferred())

    d = _tdefer.ensureDeferred(
        prov.fetch("local_content/x", FakeFileInfo("MEDIAID"))
    )
    # Still pending: the body read is stalled.
    prov.reactor.advance(prov.timeout_s + 1)  # trip the reactor deadline
    result = _result_of(d)
    assert result is None
    assert prov._read_breaker._failures == 1  # resolved as a failure, not wedged


def test_store_hung_open_times_out_and_resolves_breaker(monkeypatch):
    # Findings 1/5: a blocking open() on a wedged media-store mount neither
    # returns nor raises. The `store_timeout_s` reactor deadline must fire so the
    # store fails loudly (store_synchronous) with the write breaker resolved.
    prov = _make_provider(cb_fail_threshold=1)
    # The off-reactor open never completes.
    monkeypatch.setattr(
        mod, "defer_to_thread", lambda reactor, fn, *a: _tdefer.Deferred()
    )
    posted = {"n": 0}
    monkeypatch.setattr(
        mod.treq,
        "post",
        lambda *a, **k: posted.__setitem__("n", posted["n"] + 1)
        or _aval(FakeResponse(201)),
    )

    d = _tdefer.ensureDeferred(
        prov.store_file("local_content/x", FakeFileInfo("m"))
    )
    prov.reactor.advance(prov.store_timeout_s + 1)  # trip the reactor deadline
    with pytest.raises(_TimeoutError):
        _result_of(d)
    assert posted["n"] == 0  # never reached the post (open hung)
    assert prov._write_breaker.state == _OPEN  # resolved as a failure, not wedged


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


def test_responder_write_to_consumer_streams_bytes_to_consumer():
    # The responder streams bytes through; it deliberately does NOT touch the
    # breaker (the read breaker is resolved in fetch at response-received).
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
    # Findings 2/3/8: a SYNCHRONOUS deliverBody raise must (a) errback `finished`
    # so the caller does not hang, and (b) leave `_streamed` False so `__exit__`
    # aborts/releases the unbuffered connection. Driven INSIDE `with` so __exit__
    # runs. It must NOT feed the breaker (setup failure, not a backend event).
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

    breaker = _CircuitBreaker(fail_threshold=5, reset_timeout=10)
    responder = _FileServiceResponder(response, on_stream_failure=breaker.on_failure)
    consumer = FakeConsumer()

    errors = {}
    with responder:
        d = responder.write_to_consumer(consumer)
        d.addErrback(lambda f: errors.__setitem__("err", f) or None)

    assert "err" in errors  # errback fired synchronously, caller does not hang
    # __exit__ aborted the connection (delivered to the aborting protocol).
    assert isinstance(response.delivered_to, _DrainAndAbort)
    assert response.transport.stopped is True
    # A setup failure is not a backend mid-stream event.
    assert breaker._failures == 0


def test_responder_records_failure_on_mid_stream_truncation():
    # Finding 4: content-GET 200 then an abnormal mid-stream end (truncation/RST)
    # records exactly one read failure so a truncating backend is shed.
    breaker = _CircuitBreaker(fail_threshold=5, reset_timeout=10)
    response = FakeResponse(200, body=b"CONTENT")
    responder = _FileServiceResponder(response, on_stream_failure=breaker.on_failure)
    consumer = FakeConsumer()

    with responder:
        d = responder.write_to_consumer(consumer)
        d.addErrback(lambda f: None)  # consume the propagated failure
        sink = response.delivered_to
        sink.dataReceived(b"CON")  # partial body
        sink.connectionLost(Failure(RuntimeError("truncated")))  # abnormal end

    assert breaker._failures == 1


def test_responder_clean_stream_records_no_breaker_mutation():
    # Finding 4: a NORMAL full completion records nothing (success already booked
    # at response-received). A pre-existing failure count is left untouched.
    breaker = _CircuitBreaker(fail_threshold=5, reset_timeout=10)
    breaker.on_failure()  # pre-existing failure = 1
    response = FakeResponse(200, body=b"CONTENT")
    responder = _FileServiceResponder(response, on_stream_failure=breaker.on_failure)
    consumer = FakeConsumer()

    with responder:
        responder.write_to_consumer(consumer)
        sink = response.delivered_to
        sink.dataReceived(b"CONTENT")
        sink.connectionLost(None)  # clean completion

    assert breaker._failures == 1  # unchanged


def test_repeated_truncations_open_breaker():
    # Finding 4: response-received (on_response) does not reset in CLOSED, so
    # repeated 200-then-truncate cycles accumulate failures and OPEN the breaker.
    breaker = _CircuitBreaker(fail_threshold=3, reset_timeout=10)
    for _ in range(3):
        breaker.on_response()  # content-200 (CLOSED: no-op, does not reset)
        response = FakeResponse(200, body=b"X")
        responder = _FileServiceResponder(
            response, on_stream_failure=breaker.on_failure
        )
        consumer = FakeConsumer()
        with responder:
            d = responder.write_to_consumer(consumer)
            d.addErrback(lambda f: None)
            sink = response.delivered_to
            sink.connectionLost(Failure(RuntimeError("truncated")))
    assert breaker.state == _OPEN


def test_responder_exit_without_stream_aborts_connection():
    # Finding 2: entering then exiting WITHOUT streaming must release/abort the
    # unbuffered connection (pool safety) — independent of the breaker.
    response = FakeResponse(200, body=b"CONTENT")
    responder = _FileServiceResponder(response)

    with responder:
        pass  # Synapse never calls write_to_consumer (client disconnect)

    # The unconsumed body was delivered to the aborting protocol, which stopped
    # the transport (connection released, not leaked).
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

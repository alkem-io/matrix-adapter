# Copyright 2026 Alkemio Foundation
# SPDX-License-Identifier: EUPL-1.2

"""
Unit tests for FileServiceStorageProvider (013-matrix-media-file-service).

Run from the repo root:

    make test-python

or directly (needs pytest + twisted + treq; Synapse itself is NOT required):

    pip install -r synapse-modules/requirements-dev.txt
    pytest synapse-modules

The treq HTTP layer and Synapse's threadpool/deferred helpers are monkeypatched,
so no live file-service or reactor is required. `conftest.py` beside this file
registers minimal `sys.modules` stand-ins for the four Synapse symbols the module
imports at load time, but ONLY when Synapse is genuinely absent — inside the
matrixdotorg/synapse image the real package is used and nothing is shimmed. The
production import path is untouched either way.

The suite is run by the `python-modules` job in .github/workflows/ci-test.yml.
"""

import io
import json
import logging
import re
import threading
import types

import pytest

from twisted.internet import defer
from twisted.internet.defer import TimeoutError as _TimeoutError
from twisted.internet.task import Clock, Cooperator as TaskCooperator
from twisted.python.failure import Failure
from twisted.internet.error import ConnectionDone
from twisted.web.client import FileBodyProducer, ResponseDone
from twisted.web.http import PotentialDataLoss
from twisted.web.iweb import UNKNOWN_LENGTH

import treq
from treq.client import _convert_files, _convert_params
from treq.multipart import MultiPartProducer

import alkemio_fileservice_provider as mod
from alkemio_fileservice_provider import (
    FileServiceStorageProvider,
    _ConsumerSink,
    _DrainAndAbort,
    _FileServiceResponder,
    _ShortBody,
)

# Captured BEFORE the autouse `_isolate_cache_pool` fixture can replace it, so
# the lifecycle tests can drive the genuine factory while every other test gets
# the harmless stand-in.
_REAL_CACHE_FILE_POOL = mod._cache_file_pool


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
    """Stand-in for Twisted's response-body TransportProxyProducer.

    Records the full IPushProducer triple, because `_ConsumerSink` now registers
    ITSELF with the consumer and proxies to this transport — so these counters
    are what prove the proxy still delivers real backpressure.
    """

    def __init__(self):
        self.stopped = False
        self.paused = 0
        self.resumed = 0

    def stopProducing(self):
        self.stopped = True

    def pauseProducing(self):
        self.paused += 1

    def resumeProducing(self):
        self.resumed += 1


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

    `length` mirrors twisted's `IResponse.length`: the int Content-Length when the
    server framed the body, otherwise the `UNKNOWN_LENGTH` SENTINEL (a str, which
    is exactly what twisted leaves there for a chunked or gzip-decoded body). It
    DEFAULTS to unknown, so every pre-existing test exercises the
    no-Content-Length path and any regression there shows up suite-wide.
    """

    def __init__(self, code, auto_body=None, stall_body=False, length=UNKNOWN_LENGTH):
        self.code = code
        self._auto_body = auto_body
        self._stall_body = stall_body
        self.delivered_to = None
        self.transport = None
        self.length = length

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

    def __init__(self, reject_producer=False, reject_unregister=False):
        self.data = bytearray()
        self.producer = None
        self.streaming = None
        self.unregistered = False
        self._reject_producer = reject_producer
        # Models the teardown race: a consumer that finished/closed FIRST (a
        # Synapse BackgroundFileConsumer whose file is closed, a twisted Request
        # already finished) raises when we try to unregister the producer.
        self._reject_unregister = reject_unregister

    def registerProducer(self, producer, streaming):
        if self._reject_producer:
            raise RuntimeError("consumer refuses a producer")
        self.producer = producer
        self.streaming = streaming

    def unregisterProducer(self):
        if self._reject_unregister:
            raise RuntimeError("consumer already torn down")
        self.unregistered = True

    def write(self, data):
        self.data += data


class FakeFile:
    """Stand-in for a streamed file handle (asserts it is streamed, not buffered)."""

    _FAKE_FD = 4242  # sentinel fd for os.fstat (stubbed in the autouse fixture)

    def __init__(self, data=b"RAWBYTES"):
        self._data = data
        self.closed = False

    def read(self, n=-1):
        return self._data

    def fileno(self):
        return self._FAKE_FD

    def close(self):
        self.closed = True


class _RealBytesFile(io.BytesIO):
    """A genuinely readable cache-file handle that also answers `fileno()`.

    `store_file` fstats the OPEN fd to size the upload deadline, so a plain
    BytesIO (whose `fileno()` raises) cannot stand in where the real producer
    has to read real bytes.
    """

    def fileno(self):
        return FakeFile._FAKE_FD


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


# --- real-treq multipart serialisation -------------------------------------


class _ImmediateCall:
    """Minimal IDelayedCall stand-in for the synchronous Cooperator below."""

    def __init__(self):
        self.cancelled = False

    def cancel(self):
        self.cancelled = True


def _sync_cooperator():
    """A Cooperator whose ticks are queued and pumped synchronously.

    treq's producers schedule their chunked reads on twisted's global Cooperator,
    which needs a RUNNING reactor. Both `MultiPartProducer` and
    `FileBodyProducer` take `cooperator` as a public constructor parameter, so
    substituting a pumpable one lets the REAL serialiser run to completion in a
    plain synchronous test. Only the SCHEDULING is replaced — every byte of the
    body is produced by treq's own code.

    Returns (cooperator, pump); pump() drains queued ticks until none remain.
    """
    queue = []

    def scheduler(tick):
        queue.append(tick)
        return _ImmediateCall()

    def pump(limit=10000):
        for _ in range(limit):
            if not queue:
                return
            queue.pop(0)()
        raise AssertionError("multipart production did not terminate")

    return TaskCooperator(scheduler=scheduler), pump


class _BodyCollector:
    """IConsumer stand-in that accumulates the produced request body."""

    def __init__(self):
        self.value = b""

    def write(self, data):
        self.value += data

    def registerProducer(self, producer, streaming):
        pass

    def unregisterProducer(self):
        pass


def _serialise_treq_multipart(data, files, file_bytes):
    """Serialise `data`/`files` exactly as treq would, returning the body bytes.

    Uses treq's own `_convert_params` / `_convert_files` / `MultiPartProducer`,
    so field ordering, headers and boundaries are the library's, not the test's.
    The only substitution is the cooperator (see `_sync_cooperator`) and a real
    seekable handle for the file part, because the module's stand-in handle is
    not readable by twisted's FileBodyProducer.
    """
    coop, pump = _sync_cooperator()
    name, _stream = files["file"]
    real_files = {
        "file": (
            name,
            "application/octet-stream",
            FileBodyProducer(io.BytesIO(file_bytes), cooperator=coop),
        )
    }
    fields = list(_convert_params(data)) + list(_convert_files(real_files))
    producer = MultiPartProducer(fields, boundary=b"BOUNDARY", cooperator=coop)

    collector = _BodyCollector()
    done = producer.startProducing(collector)
    pump()
    assert done.called, "the synchronous cooperator must drive production to completion"
    return collector.value


def _multipart_part_names(body):
    """The part names, in the order they appear in a serialised multipart body."""
    return re.findall(rb'Content-Disposition: form-data; name="([^"]+)"', body)


def _multipart_part_names_str(body):
    return [n.decode() for n in _multipart_part_names(body)]


@pytest.fixture(autouse=True)
def _isolate_cache_pool(monkeypatch):
    """Never let a test create the provider's REAL thread pool.

    `_cache_file_pool` starts OS threads and registers a reactor shutdown
    trigger; the fake reactors used throughout this suite (`task.Clock`, bare
    `object()`) support neither. Hand every caller a recording stand-in instead,
    and reset the process-wide `_cache_pool` around each test so the handful of
    tests that DO exercise the real lifecycle stay hermetic.
    """
    pool = _InlineThreadPool()
    monkeypatch.setattr(mod, "_cache_file_pool", lambda reactor: pool)
    monkeypatch.setattr(mod, "_cache_pool", None)
    return pool


@pytest.fixture(autouse=True)
def _patch_async_helpers(monkeypatch):
    # make_deferred_yieldable passes the awaitable through. defer_to_threadpool
    # runs the function synchronously and returns a (fired) Deferred — the provider
    # calls addTimeout on it, so it must be a real Deferred, not a coroutine.
    monkeypatch.setattr(mod, "make_deferred_yieldable", lambda d: d)
    monkeypatch.setattr(
        mod,
        "defer_to_threadpool",
        lambda reactor, threadpool, fn, *a: defer.execute(fn, *a),
    )
    # store_file fstats the OPEN upload fd to scale the timeout by size; the fake
    # handle's fd isn't a real inode, so default os.fstat to a small st_size
    # (max() keeps the store_timeout_s floor). Size-scaling tests override this.
    monkeypatch.setattr(
        mod.os, "fstat", lambda fd: types.SimpleNamespace(st_size=1024)
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


@pytest.mark.parametrize(
    "bad, cause_type",
    [
        ("not-a-number", ValueError),  # float() refuses the string
        (True, TypeError),             # our explicit bool reject
        (float("nan"), ValueError),    # our explicit "value is not finite"
        (10 ** 400, OverflowError),    # int too large to convert to float
    ],
)
def test_parse_config_bad_tuning_chains_the_original_cause(bad, cause_type):
    """The boot-time ValueError must CHAIN its cause (ruff B904).

    `_positive_number` collapses four distinct failures — not a number at all, a
    YAML bool, NaN/inf, and an int too large for float() — into ONE
    operator-facing message that names neither. Raising bare inside the `except`
    leaves `__cause__` unset, so the traceback reads "During handling of the
    above exception, another exception occurred", which looks like a bug in the
    validator rather than a bad homeserver.yaml value. `from err` makes the
    original the explicit cause, and that traceback is the only diagnostic an
    operator gets for a Synapse that refuses to boot.
    """
    base = {
        "file_service_url": "http://fs:4003",
        "matrix_media_bucket_id": "b",
        "timeout_s": bad,
    }
    with pytest.raises(ValueError) as excinfo:
        FileServiceStorageProvider.parse_config(base)

    assert excinfo.value.__cause__ is not None, (
        "the original exception must be chained with `from err`, not dropped"
    )
    assert isinstance(excinfo.value.__cause__, cause_type)


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
    # displayName is REQUIRED by file-service (NOT NULL column) — omitting it
    # 400s every inbound store. Below the Matrix event layer the only identifier
    # available is the opaque media_id, so that is what is sent.
    assert captured["data"]["displayName"] == "MEDIAID"
    # The metadata-before-file ordering invariant is asserted against the REAL
    # serialised body in
    # test_store_multipart_body_puts_every_metadata_part_before_the_file; the
    # dict's own key order is NOT the mechanism (treq sorts) and is deliberately
    # not asserted here.
    # Per-request timeout + unbuffered (so the reply can be drained/released).
    assert captured["timeout"] == prov.store_timeout_s
    assert captured["unbuffered"] is True
    # The body must be STREAMED from the cache handle, not a fully-buffered bytes
    # blob — and streamed by the OFF-REACTOR producer, not by treq's stock
    # `FileBodyProducer` (which reads on the reactor thread). Passing the bare
    # handle is what would silently get the stock one, via `IBodyProducer(...)`
    # adaptation inside treq's `_convert_files`.
    body = captured["files"]["file"][1]
    assert isinstance(body, mod._ThreadedFileBodyProducer)
    assert body._inputFile is fake_file
    assert not isinstance(body, (bytes, bytearray))
    # The declared length comes from the off-reactor fstat, not a reactor-side
    # seek/tell probe (the autouse fixture stubs st_size to 1024).
    assert body.length == 1024
    # Handle closed; and the 201 reply body was DRAINED (keep-alive) not aborted.
    assert fake_file.closed is True
    assert store_resp.transport.stopped is False


def test_store_multipart_body_puts_every_metadata_part_before_the_file(monkeypatch):
    """The ordering invariant file-service depends on, asserted on the WIRE bytes.

    file-service's Create handler reads the form fields BEFORE consuming the
    streamed file, so every metadata part must appear ahead of the file part in
    the serialised multipart body.

    An earlier version of this test only asserted the insertion order of the dict
    the module had just built, on the stated premise that treq preserves it. That
    premise is FALSE: treq's `_convert_params` does `list(sorted(params.items()))`,
    so the metadata parts come out ALPHABETICALLY. The invariant survives for a
    different reason — `MultiPartProducer.__init__` runs the whole field list
    through `_sorted_by_type`, which keys str/bytes values (0, name) and file
    producers (1, name), putting every string field before every file field
    however they were supplied. So it is a real guarantee, but of treq's
    serialiser, not of dict order — which is exactly why it must be asserted on
    the produced bytes.
    """
    prov = _make_provider()
    captured = {}
    store_resp = _drainable(201)

    def fake_post(url, files=None, data=None, **kw):
        captured["files"] = files
        captured["data"] = data
        return _aval(store_resp)

    monkeypatch.setattr(mod.treq, "post", fake_post)
    monkeypatch.setattr(mod, "_open_stream", lambda p: FakeFile(b"RAWBYTES"))

    _run(prov.store_file("local_content/aa/bb/MEDIAID", FakeFileInfo("MEDIAID")))

    body = _serialise_treq_multipart(captured["data"], captured["files"], b"RAWBYTES")

    # Every part the module sent must be present, and the file part LAST.
    part_names = _multipart_part_names_str(body)
    assert set(part_names) == {
        "storageBucketId",
        "externalReference",
        "displayName",
        "skipImageProcessing",
        "file",
    }
    assert part_names[-1] == "file", (
        "the file part must be serialised after every metadata part; got %r" % (part_names,)
    )
    assert part_names.index("file") == len(part_names) - 1

    # And the file's payload really is in the body, after the metadata values.
    assert b"RAWBYTES" in body
    for value in (b"00000000-0000-0000-0000-0000000000ff", b"MEDIAID", b"true"):
        assert body.index(value) < body.index(b"RAWBYTES")


def test_store_does_not_close_handle_before_the_body_send_completes(monkeypatch):
    """The cache-file handle must outlive the multipart body send.

    treq's response Deferred is NOT a "headers received" signal: twisted's
    HTTP11ClientProtocol.request chains the parser's response Deferred into the
    Deferred it returns only inside `cbRequestWritten`, i.e. after
    `Request.writeTo` completes — and for a body producer that means after the
    whole file has been read and written. So closing in `finally` after awaiting
    the post cannot truncate the upload.

    This pins the ORDERING that makes it safe: while the post Deferred is still
    pending the handle stays open, and it is closed only once that Deferred
    fires. If `store_file` ever closed the handle before/independently of the
    awaited send, this fails.
    """
    prov = _make_provider()
    fake_file = FakeFile(b"RAWBYTES")
    pending = defer.Deferred()

    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: pending)
    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)

    d = defer.ensureDeferred(
        prov.store_file("local_content/aa/bb/MEDIAID", FakeFileInfo("MEDIAID"))
    )
    assert not d.called, "the store must still be awaiting the body send"
    assert fake_file.closed is False, (
        "the handle must stay open while the multipart body is still being sent"
    )

    pending.callback(_drainable(201))  # body send finished + response headers
    _result_of(d)
    assert fake_file.closed is True, "and be closed once the send completed"


def test_store_scales_timeout_by_file_size(monkeypatch):
    # A large-but-valid upload must get proportional time: treq's timeout= bounds
    # the ENTIRE multipart upload, so a big file streaming past store_timeout_s
    # would otherwise be killed mid-stream. effective_timeout = size / throughput
    # when that exceeds the store_timeout_s floor.
    prov = _make_provider()  # store_timeout_s defaults to 30s
    captured = {}
    store_resp = _drainable(201)

    def fake_post(url, files=None, data=None, **kw):
        captured["timeout"] = kw.get("timeout")
        return _aval(store_resp)

    monkeypatch.setattr(mod.treq, "post", fake_post)
    monkeypatch.setattr(mod, "_open_stream", lambda p: FakeFile(b"x"))
    # 100 MB at 1 MB/s implies a 100s deadline — well above the 30s floor.
    big = 100 * 1_000_000
    monkeypatch.setattr(
        mod.os, "fstat", lambda fd: types.SimpleNamespace(st_size=big)
    )

    _run(prov.store_file("local_content/x", FakeFileInfo("m")))

    assert captured["timeout"] == big / mod._STORE_MIN_THROUGHPUT_BPS
    assert captured["timeout"] > prov.store_timeout_s


def test_store_small_file_uses_timeout_floor(monkeypatch):
    # A small file's size-proportional deadline is below store_timeout_s, so the
    # floor (store_timeout_s) is used.
    prov = _make_provider()
    captured = {}
    store_resp = _drainable(201)

    def fake_post(url, files=None, data=None, **kw):
        captured["timeout"] = kw.get("timeout")
        return _aval(store_resp)

    monkeypatch.setattr(mod.treq, "post", fake_post)
    monkeypatch.setattr(mod, "_open_stream", lambda p: FakeFile(b"x"))
    monkeypatch.setattr(
        mod.os, "fstat", lambda fd: types.SimpleNamespace(st_size=10)
    )  # tiny

    _run(prov.store_file("local_content/x", FakeFileInfo("m")))

    assert captured["timeout"] == prov.store_timeout_s


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
        mod, "defer_to_threadpool", lambda reactor, pool, fn, *a: defer.Deferred()
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

    def fake_dtt(reactor, pool, fn, *a):
        captured["fn"] = fn  # the guarded-open closure
        return defer.Deferred()  # never fires on its own

    monkeypatch.setattr(mod, "defer_to_threadpool", fake_dtt)
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


def test_store_guarded_open_fstat_raises_closes_handle(monkeypatch):
    # The size stat runs in the guarded open (off-reactor). If os.fstat RAISES
    # (e.g. the fd went bad), the handle must be CLOSED rather than leaked — the
    # open succeeded, so only the finally can free the FD.
    prov = _make_provider()
    fake_file = FakeFile(b"x")

    def boom_fstat(_fd):
        raise OSError("fstat failed")

    monkeypatch.setattr(mod, "_open_stream", lambda p: fake_file)
    monkeypatch.setattr(mod.os, "fstat", boom_fstat)
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(FakeResponse(201)))

    with pytest.raises(OSError):
        _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    assert fake_file.closed is True  # closed on the fstat-raise path, not leaked


def test_store_open_not_routed_through_with_timeout(monkeypatch):
    # Structural logcontext guard: the cache-file open must be awaited via
    # addTimeout DIRECTLY on the already-yieldable defer_to_threadpool Deferred,
    # never through `_with_timeout` (which adds a second make_deferred_yieldable
    # that would resume the upload under the sentinel logcontext). The autouse
    # fixture stubs make_deferred_yieldable to identity, so the wrap is invisible
    # at runtime — we spy the DEFERRED `_with_timeout` is called with and assert
    # the open's Deferred is NOT among them. This is decoupled from whichever
    # timeout constant the reply drain happens to use.
    prov = _make_provider()

    open_deferreds = []

    def capturing_dtt(reactor, pool, fn, *a):
        d = defer.execute(fn, *a)
        open_deferreds.append(d)
        return d

    seen = []
    orig = mod._with_timeout

    async def spy(reactor, timeout_s, d):
        seen.append(d)
        return await orig(reactor, timeout_s, d)

    monkeypatch.setattr(mod, "defer_to_threadpool", capturing_dtt)
    monkeypatch.setattr(mod, "_with_timeout", spy)
    monkeypatch.setattr(mod, "_open_stream", lambda p: FakeFile(b"x"))
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(_drainable(201)))

    _run(prov.store_file("local_content/x", FakeFileInfo("m")))
    assert open_deferreds  # the open went through defer_to_threadpool
    # The open's Deferred is bounded by addTimeout DIRECTLY, never via _with_timeout.
    assert all(od not in seen for od in open_deferreds)
    # ...and the guard is not vacuous — the reply DRAIN does route through it.
    assert seen


def test_store_guarded_open_runs_in_the_provider_pool(monkeypatch):
    # The guarded open is the THIRD piece of cache-file work this module hands to
    # a thread, and the first thing an upload does. It carries its own
    # `store_timeout_s` deadline, so an open merely QUEUED behind Synapse's full
    # ten-thread pool fails the upload just as surely as a queued read would.
    # It uses Synapse's `defer_to_threadpool` (not twisted's) because unlike the
    # Cooperator tick it runs under a real logcontext — but on OUR pool.
    prov = _make_provider()
    provider_pool = object()
    monkeypatch.setattr(mod, "_cache_file_pool", lambda r: provider_pool)

    seen = {}

    def capturing(reactor, threadpool, fn, *a):
        seen["pool"] = threadpool
        return defer.execute(fn, *a)

    monkeypatch.setattr(mod, "defer_to_threadpool", capturing)
    monkeypatch.setattr(mod, "_open_stream", lambda p: FakeFile(b"x"))
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(_drainable(201)))

    _run(prov.store_file("local_content/x", FakeFileInfo("m")))

    assert seen.get("pool") is provider_pool, (
        "the guarded open must not queue behind the shared reactor pool either"
    )


# --- fetch -----------------------------------------------------------------


@pytest.mark.parametrize(
    "file_info",
    [
        FakeFileInfo("MEDIAID", thumbnail=object()),
        FakeFileInfo("MEDIAID", url_cache=1),
        FakeFileInfo("MEDIAID", server_name="remote.host"),
    ],
    ids=["thumbnail", "url_cache", "remote"],
)
def test_fetch_skips_non_user_upload(monkeypatch, file_info):
    # Symmetric with store_file: a thumbnail / url-cache / remote file_info must NOT
    # hit file-service. A thumbnail file_info carries the SAME file_id as the
    # original, so a by-reference lookup would resolve to the ORIGINAL and stream
    # full-size bytes as the thumbnail (corruption). Clean miss, NO HTTP call.
    prov = _make_provider()
    called = {"get": False}
    monkeypatch.setattr(
        mod.treq, "get", lambda *a, **k: called.__setitem__("get", True)
    )
    responder = _run(prov.fetch("thumbnail/x", file_info))
    assert responder is None
    assert called["get"] is False  # never looked up by-reference


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


def test_fetch_url_encodes_media_id_and_doc_id(monkeypatch):
    # media_id (query value) and doc_id (path segment) must be percent-encoded so
    # URL-reserved chars (&, /, #, ?) can't corrupt the request URL — mirrors the
    # Go side's uuid.Parse + url.PathEscape.
    prov = _make_provider()
    calls = []

    def fake_get(url, **kw):
        calls.append(url)
        if "by-reference" in url:
            return _aval(_meta({"id": "a/b"}))
        return _aval(FakeResponse(200))

    monkeypatch.setattr(mod.treq, "get", fake_get)

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("a&b")))

    # media_id "a&b" -> "a%26b" in the ref= query value (raw & would start a new param).
    assert calls[0] == (
        "http://file-service:4003/internal/file/by-reference?ref=a%26b"
    )
    assert "ref=a&b" not in calls[0]
    # doc_id "a/b" -> "a%2Fb" in the path segment (raw / would add a path segment).
    assert calls[1] == "http://file-service:4003/internal/file/a%2Fb/content"
    assert "/a/b/content" not in calls[1]
    assert responder is not None


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


@pytest.mark.parametrize("code", [204, 302])
def test_fetch_content_non_200_is_miss_not_streamed(monkeypatch, code):
    # Strict 200: anything else reaching this code — a 2xx-non-200 (204), or a
    # 3xx that somehow arrived unfollowed — must be a MISS (drained), NOT
    # streamed as media.
    prov = _make_provider()
    content_resp = _drainable(code)
    calls = []

    def fake_get(url, **kw):
        calls.append(url)
        if "by-reference" in url:
            return _aval(_meta({"id": "doc-9"}))
        return _aval(content_resp)

    monkeypatch.setattr(mod.treq, "get", fake_get)

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None  # not streamed
    assert len(calls) == 2  # by-reference + content GET both ran
    assert content_resp.transport.stopped is False  # drained (keep-alive), not aborted


@pytest.mark.parametrize("code", [204, 302])
def test_fetch_by_reference_non_200_is_miss_not_parsed(monkeypatch, code):
    # Strict 200: a 2xx-non-200 or a 3xx redirect on the by-reference GET must be a
    # MISS (drained), NOT parsed as a doc body.
    prov = _make_provider()
    meta_resp = _drainable(code)
    calls = []

    def fake_get(url, **kw):
        calls.append(url)
        return _aval(meta_resp)

    monkeypatch.setattr(mod.treq, "get", fake_get)

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is None
    assert len(calls) == 1  # only the by-reference lookup ran (no content GET)
    assert meta_resp.transport.stopped is False  # drained (keep-alive), not parsed


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
    # bounded by DRAIN_TIMEOUT_S, so on timeout it aborts (tears the connection
    # down) instead of hanging — and fetch still returns None. This is the
    # drained-vs-aborted counterpart to the fast-drain (reused) misses above.
    #
    # It also pins WHICH deadline governs. The drain is bounded by the fixed
    # DRAIN_TIMEOUT_S, NOT by the operator-tunable `timeout_s` (the module
    # docstring and README used to claim the latter, which would make worst-case
    # miss latency scale with a knob that has nothing to do with a body we have
    # already decided to discard). The two-step advance below is what makes that
    # distinction load-bearing rather than incidental.
    prov = _make_provider()
    assert mod.DRAIN_TIMEOUT_S < prov.timeout_s, (
        "the drain deadline must be strictly tighter than the request timeout, "
        "or this test cannot tell the two apart"
    )
    stalling_miss = FakeResponse(404, stall_body=True)
    monkeypatch.setattr(mod.treq, "get", lambda url, **kw: _aval(stalling_miss))

    d = defer.ensureDeferred(prov.fetch("local_content/x", FakeFileInfo("missing")))
    prov.reactor.advance(mod.DRAIN_TIMEOUT_S - 0.1)  # just short of the drain deadline
    assert not d.called, "the drain must still be running just before DRAIN_TIMEOUT_S"
    prov.reactor.advance(0.2)  # cross it — and note this is far below timeout_s
    assert _result_of(d) is None
    assert stalling_miss.transport.stopped is True  # drain gave up -> aborted


def test_fetch_small_nonempty_reply_body_drained_and_reused(monkeypatch):
    # A miss/error reply carrying a small NON-EMPTY body (bytes fed through
    # _BodyDrainer's inherited base `_consume` discard path, then a clean close ->
    # base `_result()` None) must be DRAINED and the connection reused (NOT
    # aborted) — distinct from the oversized-abort case.
    prov = _make_provider()
    small_miss = FakeResponse(404, auto_body=b'{"error":"not found"}')  # a few bytes
    monkeypatch.setattr(mod.treq, "get", lambda url, **kw: _aval(small_miss))
    responder = _run(prov.fetch("local_content/x", FakeFileInfo("missing")))
    assert responder is None
    assert small_miss.transport.stopped is False  # small body drained -> reused


def test_fetch_oversized_reply_drain_aborts(monkeypatch):
    # A non-streamed reply body over `_MAX_DRAIN_BYTES` must abort (tear down)
    # after the cap rather than draining unbounded — and fetch still returns None.
    prov = _make_provider()
    big_miss = FakeResponse(404, auto_body=b"x" * (mod._MAX_DRAIN_BYTES + 1))
    monkeypatch.setattr(mod.treq, "get", lambda url, **kw: _aval(big_miss))
    responder = _run(prov.fetch("local_content/x", FakeFileInfo("missing")))
    assert responder is None
    assert big_miss.transport.stopped is True  # over the cap -> aborted


# --- streaming / responder (reactor code paths) ----------------------------


def test_consumer_sink_streams_with_backpressure_and_completes():
    consumer = FakeConsumer()
    finished = defer.Deferred()
    result = {}
    finished.addCallback(lambda n: result.__setitem__("written", n))

    sink = _ConsumerSink(consumer, finished)
    transport = FakeTransport()
    sink.makeConnection(transport)
    # The sink registers ITSELF as the streaming producer (it has to see
    # pause/resume to suspend the body-idle deadline) and proxies to the body
    # transport — so the consumer still gets real backpressure.
    assert consumer.producer is sink
    assert consumer.streaming is True

    sink.dataReceived(b"AB")
    sink.dataReceived(b"C")
    sink.connectionLost(None)  # clean close

    assert bytes(consumer.data) == b"ABC"
    assert consumer.unregistered is True
    assert result["written"] == 3
    assert finished.called


def test_consumer_sink_degrades_when_consumer_rejects_producer(caplog):
    consumer = FakeConsumer(reject_producer=True)
    finished = defer.Deferred()
    sink = _ConsumerSink(consumer, finished)
    with caplog.at_level(logging.WARNING, logger=mod.logger.name):
        sink.makeConnection(FakeTransport())  # must not raise
    sink.dataReceived(b"XY")
    sink.connectionLost(None)
    assert bytes(consumer.data) == b"XY"
    # The degrade must be DIAGNOSABLE: without the producer this streams with no
    # backpressure, so a slow client buffers the whole media file in memory.
    # Swallowing the rejection silently leaves an operator no way to see it.
    warnings = [r for r in caplog.records if r.levelno >= logging.WARNING]
    assert warnings, "the backpressure degrade must be logged, not swallowed"
    assert "backpressure" in warnings[0].getMessage()


# The two `unregisterProducer` teardown swallows below are the SAME race seen
# from the two ends the stream can finish at. Both are deliberate: the consumer
# that closed first has nothing left to unregister, and in both cases raising
# would be strictly worse than ignoring — it would replace an accurate outcome
# with a misleading AttributeError/RuntimeError (TTFB) or leave `finished`
# hanging forever with the media request unresolved (connectionLost). These
# tests pin the control flow those comments promise.


def test_consumer_sink_connection_lost_survives_unregister_race(caplog):
    # The consumer closed first and raises on unregisterProducer. connectionLost
    # is the ONLY place this stream's result is decided, so it must swallow that
    # and still fire `finished` — a raise here would hang the media request.
    consumer = FakeConsumer(reject_unregister=True)
    finished = defer.Deferred()
    result = {}
    finished.addCallback(lambda n: result.__setitem__("written", n))

    sink = _ConsumerSink(consumer, finished)
    sink.makeConnection(FakeTransport())
    sink.dataReceived(b"ABC")
    with caplog.at_level(logging.DEBUG, logger=mod.logger.name):
        sink.connectionLost(None)  # clean close; must not raise

    assert finished.called, "the stream result must still be delivered"
    assert result["written"] == 3
    # The swallow is justified, not invisible: it leaves a debug breadcrumb.
    assert any(
        "unregisterProducer" in r.getMessage() for r in caplog.records
    ), "the teardown swallow must leave a debug trace"


def test_consumer_sink_ttfb_timeout_survives_unregister_race():
    # Same race on the TTFB path: the errback and the transport abort must both
    # still happen, so the caller gets the accurate "content stall" failure and
    # the unbuffered connection is released rather than leaked.
    clock = Clock()
    consumer = FakeConsumer(reject_unregister=True)
    finished = defer.Deferred()
    errors = {}
    finished.addErrback(lambda f: errors.__setitem__("err", f) or None)

    sink = _ConsumerSink(consumer, finished, reactor=clock, ttfb_timeout=5.0)
    transport = FakeTransport()
    sink.makeConnection(transport)

    clock.advance(5.1)  # no first byte -> trip the deadline

    assert "err" in errors
    assert errors["err"].check(_TimeoutError), "the stall error must survive the race"
    assert transport.stopped is True, "the connection must still be aborted"


# `_ConsumerSink.connectionLost` deliberately DIVERGES from `_body_end_is_clean`
# (used by the small metadata/drain readers, which accept PotentialDataLoss). A
# streamed media body has no such luxury: PotentialDataLoss means the download
# was truncated with no clean terminator, and telling the consumer the media
# completed would serve a silently corrupt file. These pin both sides of that
# divergence, which the comment names as intentional but nothing enforced.
@pytest.mark.parametrize("clean_reason", [None, ResponseDone(), ConnectionDone()])
def test_consumer_sink_treats_clean_close_as_success(clean_reason):
    consumer = FakeConsumer()
    finished = defer.Deferred()
    outcome = {}
    finished.addCallbacks(
        lambda n: outcome.__setitem__("written", n),
        lambda f: outcome.__setitem__("err", f),
    )

    sink = _ConsumerSink(consumer, finished)
    sink.makeConnection(FakeTransport())
    sink.dataReceived(b"ABC")
    sink.connectionLost(None if clean_reason is None else Failure(clean_reason))

    assert "err" not in outcome, "a clean close must complete the media stream"
    assert outcome["written"] == 3
    assert consumer.unregistered is True


def test_consumer_sink_treats_potential_data_loss_as_failure():
    consumer = FakeConsumer()
    finished = defer.Deferred()
    outcome = {}
    finished.addCallbacks(
        lambda n: outcome.__setitem__("written", n),
        lambda f: outcome.__setitem__("err", f),
    )

    sink = _ConsumerSink(consumer, finished)
    sink.makeConnection(FakeTransport())
    sink.dataReceived(b"ABC")
    # No clean terminator: the body may be TRUNCATED.
    sink.connectionLost(Failure(PotentialDataLoss()))

    assert "written" not in outcome, (
        "a truncated media body must NOT be reported as a completed stream"
    )
    assert outcome["err"].check(PotentialDataLoss)
    assert consumer.unregistered is True
    # And the shared small-body helper still ACCEPTS it — the divergence is real,
    # not an accident of one shared predicate.
    assert mod._body_end_is_clean(Failure(PotentialDataLoss()))


# --- truncation: a short body must never look like a completed stream ------
#
# Synapse's `ensure_media_is_in_local_cache` streams provider bytes into a
# `BackgroundFileConsumer` opened on the FINAL cache path (no temp file, no
# rename, no cleanup) and its later completeness gate is `os.path.exists` alone.
# So a body that ends CLEANLY but short must be reported as an error, never as a
# completed media stream — otherwise the partial file is served as complete. See
# `_ConsumerSink`'s TRUNCATION CONTRACT for the residual, Synapse-side exposure
# the provider deliberately does not try to clean up.


def _drive_sink(expected_length, chunks, reason):
    """Run a sink to completion and return its outcome dict ({written} | {err})."""
    consumer = FakeConsumer()
    finished = defer.Deferred()
    outcome = {}
    finished.addCallbacks(
        lambda n: outcome.__setitem__("written", n),
        lambda f: outcome.__setitem__("err", f),
    )
    sink = _ConsumerSink(consumer, finished, expected_length=expected_length)
    sink.makeConnection(FakeTransport())
    for chunk in chunks:
        sink.dataReceived(chunk)
    sink.connectionLost(reason)
    return outcome, consumer


@pytest.mark.parametrize(
    "clean_reason", [None, Failure(ResponseDone()), Failure(ConnectionDone())]
)
def test_consumer_sink_short_body_against_content_length_errbacks(clean_reason):
    # (a) Fewer bytes than the declared Content-Length, delivered with a CLEAN
    # close: must errback, NOT report a completed stream.
    outcome, consumer = _drive_sink(10, [b"SHO", b"RT"], clean_reason)

    assert "written" not in outcome, (
        "a body short of its declared Content-Length must NOT be reported as a "
        "completed media stream"
    )
    assert outcome["err"].check(_ShortBody)
    assert "5" in str(outcome["err"].value) and "10" in str(outcome["err"].value)
    assert consumer.unregistered is True


def test_consumer_sink_exact_length_body_succeeds():
    # (b) Exactly the declared length still completes normally.
    outcome, consumer = _drive_sink(5, [b"AB", b"CDE"], Failure(ResponseDone()))
    assert "err" not in outcome
    assert outcome["written"] == 5
    assert consumer.unregistered is True


def test_consumer_sink_without_declared_length_still_succeeds():
    # (c) No Content-Length (chunked, or a gzip-decoded body whose header length
    # describes the COMPRESSED bytes) — the guard stands down and behaviour is
    # exactly as before: a clean close completes the stream.
    outcome, _ = _drive_sink(None, [b"ABC"], Failure(ResponseDone()))
    assert "err" not in outcome
    assert outcome["written"] == 3


def test_declared_body_length_reads_content_length_and_ignores_unknown():
    # The int Content-Length is used; twisted's UNKNOWN_LENGTH sentinel (a str,
    # what twisted leaves for a chunked body and what treq's GzipDecoder resets
    # `length` to) reads as "unknown", so a gzipped stream is never failed
    # against a compressed-byte length.
    assert mod._declared_body_length(FakeResponse(200, length=7)) == 7
    assert mod._declared_body_length(FakeResponse(200, length=0)) == 0
    assert mod._declared_body_length(FakeResponse(200, length=UNKNOWN_LENGTH)) is None
    assert mod._declared_body_length(FakeResponse(200)) is None  # defaults to unknown
    # bool is an int subclass and would otherwise compare as 0/1.
    assert mod._declared_body_length(FakeResponse(200, length=True)) is None
    assert mod._declared_body_length(object()) is None  # no `length` at all


def test_fetch_threads_content_length_into_the_stream(monkeypatch):
    # End-to-end: the content response's declared length reaches the sink, so a
    # truncated download fails the Responder Synapse is streaming from.
    prov = _make_provider()
    content = FakeResponse(200, length=7)

    def fake_get(url, **kw):
        if "by-reference" in url:
            return _aval(_meta({"id": "doc-9"}))
        return _aval(content)

    monkeypatch.setattr(mod.treq, "get", fake_get)

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    consumer = FakeConsumer()
    outcome = {}
    d = responder.write_to_consumer(consumer)
    d.addCallbacks(
        lambda n: outcome.__setitem__("written", n),
        lambda f: outcome.__setitem__("err", f),
    )

    sink = content.delivered_to
    sink.dataReceived(b"SHORT")  # 5 of the declared 7
    sink.connectionLost(Failure(ResponseDone()))  # clean close, short body

    assert "written" not in outcome
    assert outcome["err"].check(_ShortBody)


def test_fetch_full_length_stream_completes(monkeypatch):
    # ...and the same wiring completes normally when every declared byte arrives.
    prov = _make_provider()
    content = FakeResponse(200, length=7)

    def fake_get(url, **kw):
        if "by-reference" in url:
            return _aval(_meta({"id": "doc-9"}))
        return _aval(content)

    monkeypatch.setattr(mod.treq, "get", fake_get)

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    consumer = FakeConsumer()
    outcome = {}
    d = responder.write_to_consumer(consumer)
    d.addCallbacks(
        lambda n: outcome.__setitem__("written", n),
        lambda f: outcome.__setitem__("err", f),
    )

    sink = content.delivered_to
    sink.dataReceived(b"CONTENT")  # exactly 7
    sink.connectionLost(Failure(ResponseDone()))

    assert "err" not in outcome
    assert outcome["written"] == 7
    assert bytes(consumer.data) == b"CONTENT"


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


# --- content stream: time-to-first-byte (TTFB) timeout ---------------------


def test_consumer_sink_ttfb_timeout_aborts_and_errbacks():
    # 200 headers then NO body byte before the TTFB deadline: the sink aborts the
    # (unbuffered) connection and errbacks `finished` so Synapse maps the Responder
    # to a media error instead of hanging on a silent stream.
    clock = Clock()
    consumer = FakeConsumer()
    finished = defer.Deferred()
    errors = {}
    finished.addErrback(lambda f: errors.__setitem__("err", f) or None)

    sink = _ConsumerSink(consumer, finished, reactor=clock, ttfb_timeout=5.0)
    transport = FakeTransport()
    sink.makeConnection(transport)
    assert consumer.producer is sink  # sink proxies backpressure to the transport
    assert clock.getDelayedCalls()  # TTFB deadline scheduled

    clock.advance(5.1)  # no first byte -> trip the deadline

    assert "err" in errors
    assert errors["err"].check(_TimeoutError)  # a TimeoutError, not a hang
    assert transport.stopped is True  # connection aborted
    assert consumer.unregistered is True


def test_consumer_sink_first_byte_cancels_ttfb():
    # The first byte cancels the TTFB timer. This sink is built WITHOUT an
    # idle_timeout (the TTFB-only configuration), so nothing replaces it and the
    # clock may advance arbitrarily far without firing. The production wiring
    # DOES pass an idle_timeout — see the body-idle tests below — but the two
    # deadlines stay independently configurable, and this pins that a sink given
    # only a TTFB budget arms exactly one timer.
    clock = Clock()
    consumer = FakeConsumer()
    finished = defer.Deferred()
    result = {}
    finished.addCallback(lambda n: result.__setitem__("written", n))

    sink = _ConsumerSink(consumer, finished, reactor=clock, ttfb_timeout=5.0)
    sink.makeConnection(FakeTransport())
    assert clock.getDelayedCalls()  # TTFB scheduled

    sink.dataReceived(b"AB")  # first byte
    assert not clock.getDelayedCalls()  # TTFB cancelled

    clock.advance(100)  # would have fired; must NOT abort or errback now
    assert not finished.called  # still streaming (backpressure, not a stall)

    sink.dataReceived(b"C")
    sink.connectionLost(None)  # clean close
    assert result["written"] == 3
    assert bytes(consumer.data) == b"ABC"


def test_fetch_content_stream_ttfb_wired_through_responder(monkeypatch):
    # End-to-end: fetch() threads reactor + timeout_s into the responder, so a
    # 200-then-silent content stream aborts on the TTFB deadline when Synapse calls
    # write_to_consumer.
    prov = _make_provider()
    content = FakeResponse(200, stall_body=True)  # headers OK, body never delivers

    def fake_get(url, **kw):
        if "by-reference" in url:
            return _aval(_meta({"id": "doc-9"}))
        return _aval(content)

    monkeypatch.setattr(mod.treq, "get", fake_get)

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    assert responder is not None

    consumer = FakeConsumer()
    errors = {}
    d = responder.write_to_consumer(consumer)
    d.addErrback(lambda f: errors.__setitem__("err", f) or None)

    prov.reactor.advance(prov.timeout_s + 1)  # trip the TTFB deadline
    assert "err" in errors
    assert content.transport.stopped is True  # connection aborted


# --- content stream: backpressure-aware BODY-IDLE deadline -----------------
#
# The TTFB deadline alone left a one-byte-then-silent stream unbounded forever,
# and NOTHING outside this module closes that hole: Synapse bare-`await`s
# `write_to_consumer` in both `respond_with_responder` and
# `ensure_media_is_in_local_cache`; twisted DISABLES the HTTP channel idle
# timeout for the whole duration of response generation (and Synapse never
# installs one anyway); and treq's `timeout=` is cancelled the moment the
# response headers arrive, so it never covers the body. These pin the deadline
# that does close it — and, just as importantly, pin that it does NOT fire while
# a slow consumer is the reason no bytes are moving.


def _idle_sink(clock, consumer, finished, idle=5.0):
    """A sink wired the way `_FileServiceResponder` wires it: both deadlines."""
    return _ConsumerSink(
        consumer, finished, reactor=clock, ttfb_timeout=idle, idle_timeout=idle
    )


def test_consumer_sink_body_idle_timeout_fires_after_first_byte():
    # One byte then silence: the TTFB deadline is gone (it did its job), so the
    # body-idle deadline is what must bound the stream. Without it `finished`
    # never resolves and Synapse holds the request + connection forever.
    clock = Clock()
    consumer = FakeConsumer()
    finished = defer.Deferred()
    errors = {}
    finished.addErrback(lambda f: errors.__setitem__("err", f) or None)

    sink = _idle_sink(clock, consumer, finished)
    transport = FakeTransport()
    sink.makeConnection(transport)

    sink.dataReceived(b"X")  # first byte: cancels TTFB, arms body-idle
    assert not finished.called

    clock.advance(5.1)  # then silence past the idle deadline

    assert "err" in errors
    assert errors["err"].check(_TimeoutError), "a stalled body must fail, not hang"
    assert transport.stopped is True, "the unbuffered connection must be aborted"
    assert consumer.unregistered is True


def test_consumer_sink_idle_timeout_survives_unregister_race():
    # The TTFB and body-idle deadlines now share one teardown (`_fail_stalled`),
    # so the deliberate `unregisterProducer` swallow has to hold on the NEW path
    # too: a consumer that closed first must still get the accurate stall error
    # and the unbuffered connection must still be aborted rather than leaked.
    clock = Clock()
    consumer = FakeConsumer(reject_unregister=True)
    finished = defer.Deferred()
    errors = {}
    finished.addErrback(lambda f: errors.__setitem__("err", f) or None)

    sink = _idle_sink(clock, consumer, finished)
    transport = FakeTransport()
    sink.makeConnection(transport)
    sink.dataReceived(b"X")  # arm the body-idle deadline

    clock.advance(5.1)  # then silence; must not raise out of the timer

    assert "err" in errors
    assert errors["err"].check(_TimeoutError), "the stall error must survive the race"
    assert transport.stopped is True, "the connection must still be aborted"


def test_consumer_sink_body_idle_is_suspended_while_the_consumer_pauses():
    # THE reason a naive idle timer was rejected: a slow Element client pauses the
    # producer, and no bytes arrive for a long time through NO fault of
    # file-service. Suspending the deadline while paused is what makes it safe —
    # the clock only runs when the consumer is ready and file-service is silent.
    clock = Clock()
    consumer = FakeConsumer()
    finished = defer.Deferred()
    outcome = {}
    finished.addCallbacks(
        lambda n: outcome.__setitem__("written", n),
        lambda f: outcome.__setitem__("err", f),
    )

    sink = _idle_sink(clock, consumer, finished)
    transport = FakeTransport()
    sink.makeConnection(transport)
    sink.dataReceived(b"X")

    sink.pauseProducing()  # slow client: downstream backpressure
    assert transport.paused == 1, "the pause must still reach the body transport"
    clock.advance(1000)  # a very slow client indeed
    assert not finished.called, (
        "a paused consumer must NOT be mistaken for a file-service stall"
    )
    assert transport.stopped is False

    sink.resumeProducing()  # client caught up; the clock starts again
    assert transport.resumed == 1, "the resume must still reach the body transport"
    sink.dataReceived(b"Y")
    sink.connectionLost(None)

    assert "err" not in outcome
    assert outcome["written"] == 2


def test_consumer_sink_body_idle_resumes_and_still_catches_a_stall():
    # The other half of suspension: once the consumer resumes, the deadline must
    # be re-armed. A timer that suspended and never came back would silently
    # restore the original unbounded hang for any stream that paused even once.
    clock = Clock()
    consumer = FakeConsumer()
    finished = defer.Deferred()
    errors = {}
    finished.addErrback(lambda f: errors.__setitem__("err", f) or None)

    sink = _idle_sink(clock, consumer, finished)
    sink.makeConnection(FakeTransport())
    sink.dataReceived(b"X")
    sink.pauseProducing()
    clock.advance(1000)
    assert not finished.called

    sink.resumeProducing()  # ready for more, and file-service says nothing
    clock.advance(5.1)

    assert "err" in errors
    assert errors["err"].check(_TimeoutError)


def test_consumer_sink_body_idle_is_reset_by_every_chunk():
    # A healthy-but-slow-ish stream that keeps delivering inside the window must
    # never trip. Only a gap LONGER than the window is a stall, so each chunk has
    # to restart the clock rather than letting the first timer run to term.
    clock = Clock()
    consumer = FakeConsumer()
    finished = defer.Deferred()
    outcome = {}
    finished.addCallbacks(
        lambda n: outcome.__setitem__("written", n),
        lambda f: outcome.__setitem__("err", f),
    )

    sink = _idle_sink(clock, consumer, finished)
    sink.makeConnection(FakeTransport())

    # Four chunks, each 4s after the last: 16s of wall clock, never 5s idle.
    for _ in range(4):
        sink.dataReceived(b"AB")
        clock.advance(4.0)
    assert not finished.called, "chunks inside the window must reset the deadline"

    clock.advance(1.5)  # now the gap since the last chunk exceeds 5s
    assert "err" in outcome
    assert outcome["err"].check(_TimeoutError)


def test_consumer_sink_clean_close_leaves_no_pending_deadline():
    # Teardown discipline: neither deadline may outlive the stream as a pending
    # IDelayedCall. A leaked timer fires against a finished stream (harmless only
    # because `_fail_stalled` re-checks `finished.called`) and pins the sink, the
    # consumer and the response in memory until it does.
    clock = Clock()
    consumer = FakeConsumer()
    finished = defer.Deferred()

    sink = _idle_sink(clock, consumer, finished)
    sink.makeConnection(FakeTransport())
    sink.dataReceived(b"ABC")
    assert clock.getDelayedCalls(), "the body-idle deadline should be armed"

    sink.connectionLost(None)  # clean close

    assert finished.called
    assert clock.getDelayedCalls() == [], "connectionLost must cancel both deadlines"


def test_consumer_sink_stop_producing_tears_down_and_cancels_deadlines():
    # A client disconnect reaches us as HTTPChannel.stopProducing() -> ours (that
    # is the ONLY external escape from a stalled stream, and it only exists
    # because we register a producer). It must abort the upstream connection and
    # leave no timer behind. It must NOT resolve `finished` itself —
    # connectionLost stays the single place this stream's result is decided.
    clock = Clock()
    consumer = FakeConsumer()
    finished = defer.Deferred()

    sink = _idle_sink(clock, consumer, finished)
    transport = FakeTransport()
    sink.makeConnection(transport)
    sink.dataReceived(b"X")

    sink.stopProducing()

    assert transport.stopped is True, "the upstream connection must be aborted"
    assert clock.getDelayedCalls() == [], "no deadline may survive the teardown"
    assert not finished.called, "connectionLost decides the result, not stopProducing"

    sink.connectionLost(Failure(ConnectionDone()))
    assert finished.called


def test_fetch_content_stream_idle_deadline_wired_through_responder(monkeypatch):
    # End-to-end: fetch() must thread the stall budget into BOTH deadlines, not
    # just TTFB. A content stream that delivers one byte and then goes silent has
    # to fail through the real fetch() -> Responder -> sink wiring.
    prov = _make_provider()
    content = FakeResponse(200, stall_body=True)

    def fake_get(url, **kw):
        if "by-reference" in url:
            return _aval(_meta({"id": "doc-9"}))
        return _aval(content)

    monkeypatch.setattr(mod.treq, "get", fake_get)

    responder = _run(prov.fetch("local_content/x", FakeFileInfo("MEDIAID")))
    consumer = FakeConsumer()
    errors = {}
    d = responder.write_to_consumer(consumer)
    d.addErrback(lambda f: errors.__setitem__("err", f) or None)

    # One byte: this is what used to disarm every deadline the module had.
    content.delivered_to.dataReceived(b"X")
    assert not errors

    prov.reactor.advance(prov.timeout_s + 1)  # then silence

    assert "err" in errors, "a one-byte-then-silent stream must not hang forever"
    assert errors["err"].check(_TimeoutError)
    assert content.transport.stopped is True


# --- store_file: OFF-REACTOR cache-file reads ------------------------------
#
# treq streams the upload through twisted's stock `FileBodyProducer`, whose
# `_writeloop` calls `inputFile.read()` ON THE REACTOR THREAD. A blocked reactor
# cannot run `callLater`, so the store's own size-scaled `addTimeout` deadline
# could not fire either — the guard meant to bound a wedged media-store read is
# disarmed by exactly the thing it guards. The mainline synapse-s3-storage-provider
# makes no such trade-off: its store_file hands the whole boto3 upload (file read
# included) to a threadpool. These pin the adaptation that closes the gap here,
# and pin that it does NOT cost the backpressure the base class provides.


class _RecordingThreadBridge:
    """Stand-in for `_read_in_thread`: records each dispatch, runs it inline."""

    def __init__(self):
        self.calls = 0

    def __call__(self, reactor, fn):
        self.calls += 1
        return defer.execute(fn)


class _InlineThreadPool:
    """Default provider-pool stand-in (see `_isolate_cache_pool`): runs work NOW.

    Before the provider owned a pool, the suite's `Clock` reactor exposed none
    and `_close_off_reactor` therefore closed inline. Running dispatched work
    immediately preserves that for every test that only cares the handle ends up
    closed. Tests that assert work was genuinely DEFERRED to a pool substitute
    `_RecordingThreadPool` instead.
    """

    def __init__(self):
        self.ran = []

    def callInThread(self, fn, *args, **kwargs):
        self.ran.append(fn)
        fn(*args, **kwargs)


class _RecordingThreadPool:
    """Stand-in for the PROVIDER's thread pool that DEFERS the work it is handed.

    Deferring is the point: it is what distinguishes "dispatched to the pool"
    from "run inline on the calling (reactor) thread".
    """

    def __init__(self):
        self.queued = []

    def callInThread(self, fn, *args, **kwargs):
        self.queued.append(lambda: fn(*args, **kwargs))

    def run(self):
        while self.queued:
            self.queued.pop(0)()


def test_threaded_body_producer_reads_off_reactor_and_preserves_the_bytes(monkeypatch):
    # Every chunk read must go through the off-reactor seam, and the body treq
    # ends up sending must be byte-identical to the file.
    coop, pump = _sync_cooperator()
    bridge = _RecordingThreadBridge()
    monkeypatch.setattr(mod, "_read_in_thread", bridge)

    payload = b"0123456789" * 7  # 70 bytes
    producer = mod._ThreadedFileBodyProducer(
        io.BytesIO(payload),
        reactor=object(),  # the autouse fixture supplies the provider pool
        length=len(payload),
        cooperator=coop,
        read_size=16,
    )
    collector = _BodyCollector()
    done = producer.startProducing(collector)
    pump()

    assert done.called
    assert collector.value == payload, "the streamed body must be verbatim"
    # 70 bytes at 16 per read = 5 full reads, then one more that returns b"" (EOF).
    assert bridge.calls == 6, (
        "every read must be dispatched off-reactor; got %d" % bridge.calls
    )
    # The declared length is the one we were given (fstat'd off-reactor), not a
    # reactor-side seek/tell probe.
    assert producer.length == len(payload)


def test_threaded_body_producer_still_honours_pause_and_resume(monkeypatch):
    # Moving the reads into the thread pool must not cost backpressure. The
    # mechanism is the base class's untouched CooperativeTask, so a paused
    # producer must read NOTHING — not even one chunk ahead into memory.
    coop, pump = _sync_cooperator()
    bridge = _RecordingThreadBridge()
    monkeypatch.setattr(mod, "_read_in_thread", bridge)

    payload = b"ABCDEFGH"
    producer = mod._ThreadedFileBodyProducer(
        io.BytesIO(payload),
        reactor=object(),
        length=len(payload),
        cooperator=coop,
        read_size=4,
    )
    collector = _BodyCollector()
    done = producer.startProducing(collector)

    producer.pauseProducing()  # slow consumer
    pump()
    assert collector.value == b"", "a paused producer must not write"
    assert bridge.calls == 0, "a paused producer must not even read"
    assert not done.called

    producer.resumeProducing()
    pump()
    assert done.called
    assert collector.value == payload


def test_threaded_body_producer_stop_closes_the_handle_off_the_reactor(monkeypatch):
    # The base class closes the handle INLINE and BEFORE stopping the task.
    # io.BufferedReader.close() acquires the object's buffer lock, so closing
    # while a chunk read is still in flight in the pool blocks the caller until
    # that read returns — on the reactor thread that is the very stall the
    # threaded reads exist to prevent, merely relocated into teardown.
    coop, _pump = _sync_cooperator()
    monkeypatch.setattr(mod, "_read_in_thread", _RecordingThreadBridge())
    pool = _RecordingThreadPool()
    monkeypatch.setattr(mod, "_cache_file_pool", lambda r: pool)
    reactor = object()

    handle = io.BytesIO(b"AB")
    producer = mod._ThreadedFileBodyProducer(
        handle, reactor, length=2, cooperator=coop, read_size=2
    )
    producer.startProducing(_BodyCollector())

    producer.stopProducing()

    assert handle.closed is False, (
        "the handle must NOT be closed on the calling (reactor) thread"
    )
    assert pool.queued, "the close must be dispatched to the provider thread pool"
    pool.run()
    assert handle.closed is True, "and must actually happen once the pool runs it"

    # Stopping an already-stopped task raises TaskFinished; the override must
    # swallow it exactly as the base class does, or a cancelled upload's teardown
    # would blow up instead of releasing the handle.
    producer.stopProducing()


def test_read_in_thread_dispatches_to_the_provider_pool_not_the_reactors(monkeypatch):
    # The seam every cache-file read passes through must really hand the call to
    # a thread pool. If it ever degraded to running inline, the producer above
    # would silently be back to reading on the reactor thread and every test that
    # substitutes the seam would still pass.
    #
    # And it must be the PROVIDER's pool. The shared reactor pool is
    # `ThreadPool(0, 10)` and Synapse never resizes it, while
    # `BackgroundFileConsumer._writer` parks one of those ten on an untimed
    # `Queue.get()` for the whole duration of every `ensure_media_is_in_local_cache`
    # transfer — i.e. of our own `fetch`, which a thumbnail of any purged upload
    # takes. A read QUEUED behind that still burns `effective_timeout`, so a
    # healthy upload would fail on a timeout. See `_cache_file_pool`.
    seen = {}

    def fake_defer_to_thread_pool(reactor, pool, fn):
        seen["reactor"], seen["pool"], seen["fn"] = reactor, pool, fn
        return defer.succeed(b"chunk")

    monkeypatch.setattr(mod.threads, "deferToThreadPool", fake_defer_to_thread_pool)

    provider_pool = object()
    monkeypatch.setattr(mod, "_cache_file_pool", lambda r: provider_pool)
    reactor_pool = object()
    reactor = types.SimpleNamespace(getThreadPool=lambda: reactor_pool)

    def read():
        return b"x"

    result = _result_of(mod._read_in_thread(reactor, read))

    assert seen.get("reactor") is reactor
    assert seen.get("pool") is provider_pool, "the PROVIDER's own pool must be used"
    assert seen.get("pool") is not reactor_pool, (
        "never the shared reactor pool — that is the starvation this fixes"
    )
    assert seen.get("fn") is read
    assert result == b"chunk"


def test_close_off_reactor_dispatches_to_the_provider_pool(monkeypatch):
    # The close is the tail of the same upload and blocks on the handle's buffer
    # lock behind any in-flight read, so on a wedged mount it strands a thread for
    # as long as that read does. It must strand one of OURS, not one of Synapse's
    # ten.
    pool = _RecordingThreadPool()
    monkeypatch.setattr(mod, "_cache_file_pool", lambda r: pool)
    reactor_pool = _RecordingThreadPool()
    reactor = types.SimpleNamespace(getThreadPool=lambda: reactor_pool)

    handle = io.BytesIO(b"x")
    mod._close_off_reactor(reactor, handle)

    assert not reactor_pool.queued, "the shared reactor pool must not be used"
    assert pool.queued, "the close must go to the provider's pool"
    assert handle.closed is False, "and must not happen on the calling thread"
    pool.run()
    assert handle.closed is True


def test_close_off_reactor_falls_back_inline_when_the_pool_is_unavailable(monkeypatch):
    # Dispatch is best-effort, but a close must never be silently DROPPED — that
    # would leak the FD for every upload. If the pool cannot be obtained at all,
    # close inline rather than raise out of a teardown path.
    def no_pool(reactor):
        raise RuntimeError("no pool available")

    monkeypatch.setattr(mod, "_cache_file_pool", no_pool)

    handle = io.BytesIO(b"x")
    mod._close_off_reactor(object(), handle)

    assert handle.closed is True


# --- the provider-owned thread pool: lifecycle -----------------------------
#
# A pool is a process RESOURCE, unlike the per-request state this module
# deliberately keeps none of. That earns it a real lifecycle: created once,
# bounded, and stopped on reactor shutdown. A pool leaked on a second
# instantiation, or never stopped, would be worse than the contention it fixes
# — mainline shipped the pool without a stop and paid ~30s on every Synapse
# shutdown until it retrofitted the trigger.


class _FakeThreadPool:
    """Records construction/start/stop without spawning a single real thread."""

    instances = []

    def __init__(self, **kwargs):
        self.kwargs = kwargs
        self.started = 0
        self.stopped = 0
        _FakeThreadPool.instances.append(self)

    def start(self):
        self.started += 1

    def stop(self):
        self.stopped += 1


@pytest.fixture
def fake_pool_class(monkeypatch):
    monkeypatch.setattr(_FakeThreadPool, "instances", [])
    monkeypatch.setattr(mod, "ThreadPool", _FakeThreadPool)
    monkeypatch.setattr(mod, "_cache_pool", None)
    return _FakeThreadPool


def _trigger_recording_reactor(triggers):
    return types.SimpleNamespace(
        addSystemEventTrigger=lambda phase, event, fn: triggers.append(
            (phase, event, fn)
        )
    )


def test_cache_file_pool_is_created_once_per_process(fake_pool_class):
    # Synapse builds one provider per `media_storage_providers` entry, but nothing
    # stops a process from constructing the provider more than once (a second
    # entry, a second HomeServer, a test harness). Mainline's per-instance
    # `ThreadPool(...)` in `__init__` is unguarded and would start a second pool
    # AND register a second shutdown trigger. One pool per PROCESS.
    triggers = []
    reactor = _trigger_recording_reactor(triggers)

    first = _REAL_CACHE_FILE_POOL(reactor)
    second = _REAL_CACHE_FILE_POOL(reactor)

    assert first is second, "every caller must share the one pool"
    assert len(fake_pool_class.instances) == 1, (
        "a second call must NOT construct a second pool"
    )
    assert first.started == 1, "nor start one twice"
    assert len(triggers) == 1, "nor register a second shutdown trigger"


def test_cache_file_pool_registers_a_shutdown_trigger_that_stops_it(fake_pool_class):
    # Without this the non-daemon worker threads keep the interpreter alive and
    # Synapse shutdown hangs. ModuleApi exposes no shutdown hook, so the reactor
    # trigger is the only route available.
    triggers = []
    pool = _REAL_CACHE_FILE_POOL(_trigger_recording_reactor(triggers))

    assert len(triggers) == 1, "the pool must register a shutdown trigger"
    phase, event, fn = triggers[0]
    assert (phase, event) == ("during", "shutdown")

    assert pool.stopped == 0
    fn()  # the reactor shutting down
    assert pool.stopped == 1, "the trigger must actually stop the pool"


def test_cache_file_pool_starts_nothing_if_the_shutdown_trigger_cannot_register(
    fake_pool_class,
):
    # Ordering guard: the trigger is registered BEFORE start(). A pool that has
    # been started but can never be stopped is exactly the leak this ordering
    # exists to make impossible.
    class _NoTriggers:
        def addSystemEventTrigger(self, *args):
            raise RuntimeError("this reactor cannot register shutdown triggers")

    with pytest.raises(RuntimeError):
        _REAL_CACHE_FILE_POOL(_NoTriggers())

    assert fake_pool_class.instances, "the pool object was constructed"
    assert fake_pool_class.instances[0].started == 0, (
        "an unstoppable pool must never be STARTED"
    )
    assert mod._cache_pool is None, "and must not be cached for later callers"


def test_cache_file_pool_is_bounded_and_idles_at_zero_threads(fake_pool_class):
    # The bound is the whole point of owning a pool rather than an unbounded
    # thread-per-read. It is deliberately far below mainline's 40: that number
    # sizes for threads PARKED on `wakeup_event.wait(90)` for a whole download,
    # whereas nothing here ever parks — a thread is held for one syscall.
    _REAL_CACHE_FILE_POOL(_trigger_recording_reactor([]))
    kwargs = fake_pool_class.instances[0].kwargs

    assert kwargs["maxthreads"] == mod._CACHE_POOL_MAX_THREADS
    assert 0 < mod._CACHE_POOL_MAX_THREADS < 40, (
        "the bound must be justified by this module's never-parking reads, "
        "not copied from mainline's parking-sized 40"
    )
    assert kwargs["minthreads"] == 0, (
        "an idle Synapse must pay no threads at all (mainline keeps 5 alive "
        "forever by leaving twisted's minthreads default in place)"
    )
    assert kwargs["name"], "the pool must be named so it is identifiable in a dump"


def test_cache_file_pool_really_starts_and_stops_a_twisted_thread_pool(monkeypatch):
    # The tests above substitute ThreadPool, so none of them would catch a
    # mistyped constructor argument or a twisted API change — they never touch
    # the real class. This one builds the GENUINE pool, runs work on a genuine
    # worker thread, and stops it through the registered trigger. It is the only
    # test in this suite that spawns a thread, and it joins it before returning.
    monkeypatch.setattr(mod, "_cache_pool", None)
    triggers = []
    pool = _REAL_CACHE_FILE_POOL(_trigger_recording_reactor(triggers))

    try:
        assert pool.started is True
        assert (pool.min, pool.max) == (0, mod._CACHE_POOL_MAX_THREADS)

        ran = threading.Event()
        where = []
        pool.callInThread(
            lambda: (where.append(threading.current_thread()), ran.set())
        )
        assert ran.wait(10), "the real pool must actually run dispatched work"
        assert where[0] is not threading.current_thread(), (
            "...on a worker thread, never the caller's"
        )
    finally:
        _phase, _event, stop = triggers[0]
        stop()  # the reactor shutting down, for real this time
        stopped_by_trigger = pool.started is False
        workers_joined = all(not t.is_alive() for t in pool.threads)
        if not stopped_by_trigger:
            # Report the regression as a FAILURE, never as a hung test run: a
            # live non-daemon worker would keep the interpreter (and so pytest)
            # alive forever — which is precisely the Synapse-shutdown hang this
            # trigger exists to prevent.
            pool.stop()

    assert stopped_by_trigger, "the shutdown trigger must really stop the pool"
    assert workers_joined, (
        "and must JOIN every worker — an unjoined non-daemon thread is exactly "
        "what hangs Synapse shutdown"
    )


def test_store_body_serialises_through_real_treq_with_the_threaded_producer(monkeypatch):
    """The threaded producer must survive treq's REAL `files` handling.

    `_convert_files` adapts whatever it is handed with `IBodyProducer(...)`: a
    bare file handle becomes twisted's stock, reactor-reading `FileBodyProducer`,
    whereas an object that already provides `IBodyProducer` is passed through
    untouched. Everything else in this suite substitutes the read seam, so this
    is the one place that proves the swap actually works against the library —
    that treq keeps our producer, keeps the same part filename and content type
    as when it got a bare handle, and still frames the body with an EXACT
    declared length (a wrong length here corrupts every upload).

    The body is serialised inside the fake `post`, which is where the real one
    reads it — the cache handle is closed only after the post Deferred fires.
    """
    prov = _make_provider()
    payload = b"MEDIABYTES" * 9  # 90 bytes: many reads at the tiny read_size below
    captured = {}

    def fake_post(url, files=None, data=None, **kw):
        coop, pump = _sync_cooperator()
        fields = list(_convert_params(data)) + list(_convert_files(files))
        _name, (filename, content_type, producer) = [
            f for f in fields if f[0] == "file"
        ][0]
        captured["producer"] = producer
        captured["passthrough"] = producer is files["file"][1]
        captured["part"] = (filename, content_type)

        # Only the SCHEDULING is replaced (see `_sync_cooperator`): treq's own
        # serialiser produces every byte. The producer was built with the global
        # cooperator, which needs a running reactor.
        producer._cooperate = coop.cooperate
        multipart = MultiPartProducer(fields, boundary=b"BOUNDARY", cooperator=coop)
        collector = _BodyCollector()
        done = multipart.startProducing(collector)
        pump()
        captured["done"] = done.called
        captured["body"] = collector.value
        captured["declared_length"] = multipart.length
        return _aval(_drainable(201))

    monkeypatch.setattr(mod.treq, "post", fake_post)
    monkeypatch.setattr(mod, "_open_stream", lambda p: _RealBytesFile(payload))
    monkeypatch.setattr(
        mod.os, "fstat", lambda fd: types.SimpleNamespace(st_size=len(payload))
    )
    # Run the "thread pool" inline so the synchronous cooperator can drive it.
    monkeypatch.setattr(mod, "_read_in_thread", lambda reactor, fn: defer.execute(fn))

    _run(prov.store_file("local_content/aa/bb/MEDIAID", FakeFileInfo("MEDIAID")))

    assert captured["passthrough"], (
        "treq must pass our IBodyProducer through, not adapt the handle into its "
        "own reactor-reading FileBodyProducer"
    )
    assert isinstance(captured["producer"], mod._ThreadedFileBodyProducer)
    # Unchanged from when a bare handle was passed: treq derives both from the
    # 2-tuple's filename.
    assert captured["part"] == ("MEDIAID", "application/octet-stream")

    assert captured["done"], "treq's serialiser must run the producer to completion"
    assert payload in captured["body"], "the file bytes must reach the wire verbatim"
    assert captured["declared_length"] == len(captured["body"]), (
        "the declared multipart Content-Length must equal the bytes produced"
    )


def test_consumer_sink_no_idle_deadline_before_the_first_byte():
    # The two deadlines are strictly sequential: TTFB owns the stream until the
    # first byte, the body-idle deadline from then on. A pause/resume before any
    # data must therefore leave exactly ONE timer armed (the TTFB one), not arm a
    # second one racing it for the same condition.
    clock = Clock()
    consumer = FakeConsumer()
    finished = defer.Deferred()

    sink = _idle_sink(clock, consumer, finished)
    sink.makeConnection(FakeTransport())
    assert len(clock.getDelayedCalls()) == 1  # TTFB only

    sink.pauseProducing()
    sink.resumeProducing()
    assert len(clock.getDelayedCalls()) == 1, (
        "no body-idle deadline may be armed before the first byte"
    )

    sink.dataReceived(b"X")  # now TTFB gives way to the body-idle deadline
    assert len(clock.getDelayedCalls()) == 1

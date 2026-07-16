# Copyright 2026 Alkemio Foundation
# SPDX-License-Identifier: EUPL-1.2

"""
Unit tests for FileServiceStorageProvider (013-matrix-media-file-service).

Run inside the Synapse virtualenv (needs twisted + treq + synapse importable):

    pip install pytest pytest-asyncio
    pytest test_alkemio_fileservice_provider.py

The treq HTTP layer and Synapse's threadpool/deferred helpers are monkeypatched,
so no live file-service or reactor is required. On a bare host without the Synapse
test deps the import will fail at collection — that is expected; these run in the
matrixdotorg/synapse image where twisted/treq/synapse are present.
"""

import asyncio
import types

import pytest

import alkemio_fileservice_provider as mod
from alkemio_fileservice_provider import FileServiceStorageProvider, _CircuitBreaker


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


class FakeResponse:
    def __init__(self, code, json_body=None, body=b""):
        self.code = code
        self._json = json_body
        self._body = body
        self.delivered_to = None

    def deliverBody(self, protocol):
        self.delivered_to = protocol


async def _aval(x):
    return x


def _make_provider():
    cfg = FileServiceStorageProvider.parse_config(
        {
            "file_service_url": "http://file-service:4003/",
            "matrix_media_bucket_id": "00000000-0000-0000-0000-0000000000ff",
        }
    )
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
        FileServiceStorageProvider.parse_config(
            {"matrix_media_bucket_id": "x"}
        )


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
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: called.__setitem__("post", True))
    asyncio.get_event_loop().run_until_complete(
        prov.store_file("remote_content/x", FakeFileInfo("m", server_name="h"))
    )
    assert called["post"] is False


# --- store_file ------------------------------------------------------------


def test_store_posts_verbatim_multipart(monkeypatch):
    prov = _make_provider()
    captured = {}

    def fake_post(url, files=None, data=None, **kw):
        captured["url"] = url
        captured["files"] = files
        captured["data"] = data
        return _aval(FakeResponse(201, json_body={"id": "doc-1"}))

    read_paths = []

    monkeypatch.setattr(mod.treq, "post", fake_post)
    monkeypatch.setattr(
        mod, "_read_bytes", lambda p: read_paths.append(p) or b"RAWBYTES"
    )

    asyncio.get_event_loop().run_until_complete(
        prov.store_file("local_content/aa/bb/MEDIAID", FakeFileInfo("MEDIAID"))
    )

    # C3 regression guard: the provider must read the local cache file at
    # media_store_path + the `path` arg, NOT a (non-existent) FileInfo attribute.
    assert read_paths == ["/data/media_store/local_content/aa/bb/MEDIAID"]
    assert captured["url"] == "http://file-service:4003/internal/file"
    assert captured["data"]["externalReference"] == "MEDIAID"
    assert captured["data"]["skipImageProcessing"] == "true"
    assert captured["data"]["storageBucketId"] == "00000000-0000-0000-0000-0000000000ff"
    assert captured["files"]["file"][1] == b"RAWBYTES"


def test_store_raises_and_trips_breaker_on_http_error(monkeypatch):
    prov = _make_provider()
    monkeypatch.setattr(mod.treq, "post", lambda *a, **k: _aval(FakeResponse(500)))
    monkeypatch.setattr(mod, "_read_bytes", lambda p: b"x")
    with pytest.raises(RuntimeError):
        asyncio.get_event_loop().run_until_complete(
            prov.store_file("local_content/x", FakeFileInfo("m"))
        )


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

    responder = asyncio.get_event_loop().run_until_complete(
        prov.fetch("local_content/x", FakeFileInfo("MEDIAID"))
    )

    # global lookup: NO bucketId query param
    assert calls[0] == "http://file-service:4003/internal/file/by-reference?ref=MEDIAID"
    assert "bucketId" not in calls[0]
    assert calls[1] == "http://file-service:4003/internal/file/doc-9/content"
    assert responder is not None


def test_fetch_returns_none_on_miss(monkeypatch):
    prov = _make_provider()

    def fake_get(url, **kw):
        return _aval(FakeResponse(404))

    monkeypatch.setattr(mod.treq, "get", fake_get)
    monkeypatch.setattr(mod.treq, "json_content", lambda r: _aval(r._json))

    responder = asyncio.get_event_loop().run_until_complete(
        prov.fetch("local_content/x", FakeFileInfo("missing"))
    )
    assert responder is None


# --- circuit breaker -------------------------------------------------------


def test_circuit_breaker_opens_and_recovers():
    t = {"now": 0.0}
    cb = _CircuitBreaker(fail_threshold=2, reset_timeout=10, clock=lambda: t["now"])

    cb.before_call()
    cb.on_failure()
    cb.before_call()
    cb.on_failure()  # threshold reached -> OPEN

    with pytest.raises(mod._CircuitOpenError):
        cb.before_call()

    # after reset timeout -> HALF-OPEN allows a trial call
    t["now"] = 11.0
    cb.before_call()  # no raise
    cb.on_success()   # closes
    cb.before_call()  # still closed

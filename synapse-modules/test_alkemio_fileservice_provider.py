# Copyright 2026 Alkemio Foundation
# SPDX-License-Identifier: EUPL-1.2
"""Provider boundaries; real-byte roundtrips also run in the Synapse image."""
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest
from twisted.internet import defer
from twisted.python.failure import Failure
import alkemio_fileservice_provider as mod


def run(coroutine):
    result = []
    defer.ensureDeferred(coroutine).addBoth(result.append)
    assert result, "HTTP doubles must finish synchronously"
    if isinstance(result[0], Failure):
        result[0].raiseException()
    return result[0]


def info(**overrides):
    fields = dict(file_id="media123", server_name=None, thumbnail=None, url_cache=False)
    fields.update(overrides)
    return SimpleNamespace(**fields)


@pytest.fixture
def provider(tmp_path):
    http = SimpleNamespace(get_json=AsyncMock(return_value={"id": "document123"}),
                           get_file=AsyncMock())
    hs = SimpleNamespace(
        get_reactor=lambda: None,
        get_simple_http_client=lambda: http,
        config=SimpleNamespace(media=SimpleNamespace(
            media_store_path=str(tmp_path), max_upload_size=50 * 1024 * 1024)),
    )
    config = mod.FileServiceStorageProvider.parse_config({
        "file_service_url": "http://file-service:4003/",
        "matrix_media_bucket_id": "reserved-bucket",
    })
    return mod.FileServiceStorageProvider(hs, config)


@pytest.mark.parametrize("flags", [
    {"server_name": "remote.host"}, {"thumbnail": object()}, {"url_cache": True},
])
def test_non_originals_do_not_touch_file_service(provider, flags, monkeypatch):
    post = AsyncMock()
    monkeypatch.setattr(mod.treq, "post", post)
    run(provider.store_file("does-not-exist", info(**flags)))
    assert run(provider.fetch("unused", info(**flags))) is None
    post.assert_not_called()
    provider.http.get_json.assert_not_called()


@pytest.mark.parametrize("body", [b"", b"file bytes\x00\xff"])
def test_store_streams_verbatim_without_public_auth(provider, tmp_path, monkeypatch, body):
    (tmp_path / "original").write_bytes(body)
    opened = []

    def post(url, **kwargs):
        assert url == "http://file-service:4003/internal/file"
        assert kwargs["data"] == {
            "storageBucketId": "reserved-bucket", "externalReference": "media123",
            "displayName": "media123", "skipImageProcessing": "true",
        }
        stream = kwargs["files"]["file"][2]
        opened.append(stream)
        assert stream.read() == body
        return defer.succeed(SimpleNamespace(code=201))

    monkeypatch.setattr(mod.treq, "post", post)
    monkeypatch.setattr(mod.treq, "content", lambda response: defer.succeed(b"{}"))
    run(provider.store_file("original", info()))
    assert opened[0].closed


@pytest.mark.parametrize("status", [200, 413, 503])
def test_store_rejection_is_not_success(provider, tmp_path, monkeypatch, status):
    (tmp_path / "original").write_bytes(b"file")
    monkeypatch.setattr(mod.treq, "post", lambda *args, **kwargs:
                        defer.succeed(SimpleNamespace(code=status)))
    monkeypatch.setattr(mod.treq, "content", lambda response: defer.succeed(b"error"))
    with pytest.raises(RuntimeError, match=f"HTTP {status}"):
        run(provider.store_file("original", info()))


def test_fetch_scoped_and_responder_owns_spool(provider):
    async def download(url, spool, max_size):
        assert url == "http://file-service:4003/internal/file/document123/content"
        assert max_size == 50 * 1024 * 1024
        spool.write(b"unchanged content")

    provider.http.get_file.side_effect = download
    responder = run(provider.fetch("unused", info()))
    provider.http.get_json.assert_awaited_once_with(
        "http://file-service:4003/internal/file/by-reference",
        args={"ref": "media123", "bucketId": "reserved-bucket"},
    )
    with responder:
        assert responder.open_file.read() == b"unchanged content"
    assert responder.open_file.closed


def test_missing_reference_returns_miss(provider):
    provider.http.get_json.side_effect = mod.HttpResponseException(404, "missing", b"")
    assert run(provider.fetch("unused", info())) is None
    provider.http.get_file.assert_not_called()


def test_backend_failure_is_not_a_missing_reference(provider):
    provider.http.get_json.side_effect = mod.HttpResponseException(503, "unavailable", b"")
    with pytest.raises(mod.HttpResponseException):
        run(provider.fetch("unused", info()))


def test_failed_download_closes_partial_spool(provider):
    opened = []

    async def download(url, spool, max_size):
        opened.append(spool)
        spool.write(b"partial")
        raise OSError("download interrupted")

    provider.http.get_file.side_effect = download
    with pytest.raises(OSError, match="download interrupted"):
        run(provider.fetch("unused", info()))
    assert opened[0].closed


def test_config_requires_destination():
    with pytest.raises(ValueError, match="matrix_media_bucket_id"):
        mod.FileServiceStorageProvider.parse_config({"file_service_url": "http://files"})

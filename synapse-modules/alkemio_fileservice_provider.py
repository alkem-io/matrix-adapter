# Copyright 2026 Alkemio Foundation
# SPDX-License-Identifier: EUPL-1.2

"""
Alkemio file-service media storage provider for Synapse.

Workspace feature: 013-matrix-media-file-service
Contract: specs/013-matrix-media-file-service/contracts/synapse-storage-provider.md

A thin, STATELESS byte bridge between Synapse's media byte I/O and the Alkemio
file-service. Synapse's local media store is kept as an ephemeral CACHE (emptyDir);
file-service is the sole durable store.

  store_file  -> POST /internal/file (multipart) into the reserved `matrix_media`
                 bucket, VERBATIM (skipImageProcessing=true), with
                 externalReference = media_id (= file_info.file_id).
                 Routes ONLY local user uploads; thumbnails / url-cache / remote
                 media stay local-cache-only.

  fetch       -> GET /internal/file/by-reference?ref=<media_id>  (GLOBAL — no
                 bucketId, because the server may have MOVED the doc into a
                 conversation bucket during inbound re-home), then stream
                 GET /internal/file/{id}/content back through a Responder.
                 Cache miss in file-service -> return None.

The provider holds NO durable state; the media_id <-> document mapping lives on
the file-service document's opaque `externalReference`.

Deployment: copied into /data/modules (alongside alkemio_room_control.py) and
discovered via PYTHONPATH=/data/modules. Configured under
`media_storage_providers` in homeserver.yaml.

SOURCE OF TRUTH: the RUNTIME-AUTHORITATIVE copy of this module is the inline
`data:` block embedded in
`third-party/communication/synapse/01-synapse-setup-confmap.yml` — that is the
copy actually written into /data/modules at pod bootstrap. THIS standalone file
is the reference / unit-test copy (imported by
test_alkemio_fileservice_provider.py). The two MUST be kept in sync: any change
here has to be mirrored into the confmap block (and vice versa).
"""

import logging
import os
from typing import TYPE_CHECKING, Optional

from twisted.internet import defer
from twisted.internet.defer import Deferred
from twisted.internet.interfaces import IConsumer
from twisted.internet.protocol import Protocol

import treq

from synapse.logging.context import defer_to_thread, make_deferred_yieldable
from synapse.media._base import Responder
from synapse.media.storage_provider import StorageProvider

if TYPE_CHECKING:
    from synapse.media._base import FileInfo
    from synapse.server import HomeServer

logger = logging.getLogger(__name__)

# Reserved staging bucket name; the configured id is the server-seeded UUID.
MATRIX_MEDIA_BUCKET = "matrix_media"

# Conservative network timeouts (seconds). file-service is on the Element media
# read path (cache miss -> fetch), so reads must fail fast and degrade, never hang.
DEFAULT_TIMEOUT_S = 10.0
DEFAULT_STORE_TIMEOUT_S = 30.0

# Circuit-breaker defaults.
DEFAULT_CB_FAIL_THRESHOLD = 5
DEFAULT_CB_RESET_TIMEOUT_S = 30.0


class _CircuitOpenError(Exception):
    """Raised when the circuit breaker is open and is short-circuiting calls."""


class _CircuitBreaker:
    """
    Minimal in-memory circuit breaker.

    CLOSED  -> calls pass through; consecutive failures are counted.
    OPEN    -> calls are short-circuited until `reset_timeout` elapses.
    HALF    -> one trial call is allowed; success closes, failure re-opens.

    Time source is injectable for deterministic tests.
    """

    def __init__(self, fail_threshold, reset_timeout, clock=None):
        import time as _time

        self._fail_threshold = fail_threshold
        self._reset_timeout = reset_timeout
        self._clock = clock or _time.monotonic
        self._failures = 0
        self._opened_at = None  # type: Optional[float]

    def before_call(self) -> None:
        if self._opened_at is None:
            return
        if (self._clock() - self._opened_at) >= self._reset_timeout:
            # Move to HALF-OPEN: allow a single trial call through.
            self._opened_at = None
            self._failures = self._fail_threshold - 1
            logger.info("file-service circuit half-open: allowing a trial call")
            return
        raise _CircuitOpenError("file-service circuit is open")

    def on_success(self) -> None:
        if self._failures or self._opened_at is not None:
            logger.info("file-service circuit closed after success")
        self._failures = 0
        self._opened_at = None

    def on_failure(self) -> None:
        self._failures += 1
        if self._failures >= self._fail_threshold and self._opened_at is None:
            self._opened_at = self._clock()
            logger.warning(
                "file-service circuit OPEN after %d consecutive failures",
                self._failures,
            )


class _ConsumerSink(Protocol):
    """
    Twisted body protocol that forwards a streamed HTTP response body straight
    into a Synapse media consumer, firing `finished` on completion.

    Backpressure: the response-body transport is an IPushProducer, so it is
    registered with the downstream consumer as a streaming producer. A slow
    consumer (e.g. a slow Element client on the read path) can then pause/resume
    the upstream TCP read instead of forcing this protocol to buffer an unbounded
    amount of data in memory.
    """

    def __init__(self, consumer: IConsumer, finished: "Deferred[int]"):
        self._consumer = consumer
        self._finished = finished
        self._written = 0
        self._producer_registered = False

    def makeConnection(self, transport) -> None:
        Protocol.makeConnection(self, transport)
        # `transport` is Twisted's TransportProxyProducer for the response body —
        # an IPushProducer. Register it so the consumer applies real backpressure.
        # Degrade gracefully if the consumer cannot accept a producer.
        try:
            self._consumer.registerProducer(transport, True)
            self._producer_registered = True
        except (AttributeError, RuntimeError):
            self._producer_registered = False

    def dataReceived(self, data: bytes) -> None:
        self._consumer.write(data)
        self._written += len(data)

    def connectionLost(self, reason=None) -> None:
        # ResponseDone arrives here as a clean close; treat any close as "done"
        # because the consumer has already received everything streamed so far.
        from twisted.web.client import ResponseDone
        from twisted.internet.error import ConnectionDone

        if self._producer_registered:
            try:
                self._consumer.unregisterProducer()
            except (AttributeError, RuntimeError):
                pass
            self._producer_registered = False

        if self._finished.called:
            return
        if reason is None or reason.check(ResponseDone, ConnectionDone):
            self._finished.callback(self._written)
        else:
            self._finished.errback(reason)


class _FileServiceResponder(Responder):
    """
    Streams a file-service content response into the media consumer without
    buffering the whole blob in memory.
    """

    def __init__(self, response):
        self._response = response

    def write_to_consumer(self, consumer: IConsumer) -> "Deferred[int]":
        finished = defer.Deferred()  # type: Deferred[int]
        self._response.deliverBody(_ConsumerSink(consumer, finished))
        return make_deferred_yieldable(finished)

    def __exit__(self, exc_type, exc_val, exc_tb):
        # Nothing to release: the body protocol owns the connection lifecycle.
        return None


class FileServiceStorageProvider(StorageProvider):
    """
    Synapse media storage provider backed by the Alkemio file-service.

    Configured in homeserver.yaml:

        media_storage_providers:
          - module: alkemio_fileservice_provider.FileServiceStorageProvider
            store_local: true
            store_remote: false
            store_synchronous: true
            config:
              file_service_url: "http://file-service:4003"
              matrix_media_bucket_id: "<reserved uuid>"
    """

    @staticmethod
    def parse_config(config: dict) -> dict:
        """Validate and normalise the provider config block (called once at startup)."""
        if not isinstance(config, dict):
            raise ValueError(
                "FileServiceStorageProvider: config must be a mapping"
            )

        file_service_url = config.get("file_service_url")
        if not file_service_url or not isinstance(file_service_url, str):
            raise ValueError(
                "FileServiceStorageProvider: 'file_service_url' is required"
            )

        bucket_id = config.get("matrix_media_bucket_id")
        if not bucket_id or not isinstance(bucket_id, str):
            raise ValueError(
                "FileServiceStorageProvider: 'matrix_media_bucket_id' is required"
            )

        return {
            "file_service_url": file_service_url.rstrip("/"),
            "matrix_media_bucket_id": bucket_id,
            "timeout_s": float(config.get("timeout_s", DEFAULT_TIMEOUT_S)),
            "store_timeout_s": float(
                config.get("store_timeout_s", DEFAULT_STORE_TIMEOUT_S)
            ),
            "cb_fail_threshold": int(
                config.get("cb_fail_threshold", DEFAULT_CB_FAIL_THRESHOLD)
            ),
            "cb_reset_timeout_s": float(
                config.get("cb_reset_timeout_s", DEFAULT_CB_RESET_TIMEOUT_S)
            ),
        }

    def __init__(self, hs: "HomeServer", config: dict):
        self.hs = hs
        self.reactor = hs.get_reactor()
        # Local media store is now a CACHE only; kept for reference/logging.
        self.cache_path = hs.config.media.media_store_path
        self.file_service_url = config["file_service_url"]
        self.matrix_media_bucket_id = config["matrix_media_bucket_id"]
        self.timeout_s = config["timeout_s"]
        self.store_timeout_s = config["store_timeout_s"]
        self._breaker = _CircuitBreaker(
            config["cb_fail_threshold"], config["cb_reset_timeout_s"]
        )
        logger.info(
            "FileServiceStorageProvider initialized: url=%s bucket=%s cache=%s",
            self.file_service_url,
            self.matrix_media_bucket_id,
            self.cache_path,
        )

    # -- helpers ------------------------------------------------------------

    @staticmethod
    def _is_user_upload(file_info: "FileInfo") -> bool:
        """
        Offload ONLY local user uploads (path `local_content/...`).

        Thumbnails, url-cache previews and remote/federated media stay in the
        local cache and are never written to file-service.
        """
        return (
            getattr(file_info, "server_name", None) is None
            and getattr(file_info, "thumbnail", None) is None
            and not getattr(file_info, "url_cache", None)
        )

    # -- StorageProvider API ------------------------------------------------

    async def store_file(self, path: str, file_info: "FileInfo") -> None:
        """Offload a freshly-uploaded local file to file-service, verbatim."""
        if not self._is_user_upload(file_info):
            # Thumbnail / url-cache / remote: leave in local cache only.
            return

        media_id = file_info.file_id

        try:
            self._breaker.before_call()
        except _CircuitOpenError:
            # store_synchronous=true: surface so the upload fails loudly rather
            # than silently dropping the only durable copy.
            logger.error(
                "file-service circuit open; refusing to store media_id=%s", media_id
            )
            raise

        # The freshly-uploaded file lives in the local media cache at
        # media_store_path + `path` (the relative path Synapse hands us, e.g.
        # `local_content/aa/bb/<rest>`). Synapse's FileInfo has NO `upload_path`
        # attribute — derive the absolute path the same way the on-disk store does
        # (matching synapse-s3-storage-provider). Reading it is blocking I/O —
        # keep it off the reactor.
        cache_file = os.path.join(self.cache_path, path)
        body = await defer_to_thread(self.reactor, _read_bytes, cache_file)

        url = "%s/internal/file" % self.file_service_url
        # treq serialises the multipart body as form-fields (`data`) THEN files,
        # preserving dict insertion order — so storageBucketId / externalReference
        # / skipImageProcessing precede the file part, which file-service requires
        # (it reads the metadata fields before consuming the streamed file).
        files = {"file": (media_id, body)}
        data = {
            "storageBucketId": self.matrix_media_bucket_id,
            "externalReference": media_id,
            "skipImageProcessing": "true",  # VERBATIM — Synapse read-back is exact
        }

        try:
            resp = await make_deferred_yieldable(
                treq.post(
                    url,
                    files=files,
                    data=data,
                    timeout=self.store_timeout_s,
                    reactor=self.reactor,
                )
            )
        except Exception as exc:  # noqa: BLE001 - convert any transport error
            self._breaker.on_failure()
            logger.error("file-service store failed for media_id=%s: %s", media_id, exc)
            raise

        if resp.code >= 400:
            self._breaker.on_failure()
            await make_deferred_yieldable(resp.content())  # drain
            raise RuntimeError(
                "file-service store returned HTTP %d for media_id=%s"
                % (resp.code, media_id)
            )

        self._breaker.on_success()
        await make_deferred_yieldable(resp.content())  # drain so the conn is reusable
        logger.debug("Stored media_id=%s in file-service bucket=%s", media_id,
                     self.matrix_media_bucket_id)

    async def fetch(self, path: str, file_info: "FileInfo") -> Optional[Responder]:
        """
        Serve a media byte stream from file-service on local cache miss.

        Looks the document up GLOBALLY by externalReference (= media_id): the
        server may have MOVED it out of the staging bucket into a conversation
        bucket, so a bucket-scoped lookup would miss.
        """
        media_id = file_info.file_id

        try:
            self._breaker.before_call()
        except _CircuitOpenError:
            logger.warning(
                "file-service circuit open; fetch miss for media_id=%s", media_id
            )
            return None

        lookup_url = "%s/internal/file/by-reference?ref=%s" % (
            self.file_service_url,
            media_id,
        )

        try:
            meta_resp = await make_deferred_yieldable(
                treq.get(lookup_url, timeout=self.timeout_s, reactor=self.reactor)
            )
            if meta_resp.code == 404:
                self._breaker.on_success()  # a clean miss is not a fault
                await make_deferred_yieldable(meta_resp.content())
                return None
            if meta_resp.code >= 400:
                self._breaker.on_failure()
                await make_deferred_yieldable(meta_resp.content())
                logger.error(
                    "file-service by-reference HTTP %d for media_id=%s",
                    meta_resp.code,
                    media_id,
                )
                return None

            meta = await make_deferred_yieldable(treq.json_content(meta_resp))
            doc_id = meta.get("id")
            if not doc_id:
                self._breaker.on_failure()
                logger.error(
                    "file-service by-reference returned no id for media_id=%s", media_id
                )
                return None

            content_url = "%s/internal/file/%s/content" % (
                self.file_service_url,
                doc_id,
            )
            content_resp = await make_deferred_yieldable(
                treq.get(
                    content_url,
                    timeout=self.timeout_s,
                    unbuffered=True,
                    reactor=self.reactor,
                )
            )
            if content_resp.code >= 400:
                self._breaker.on_failure()
                await make_deferred_yieldable(content_resp.content())
                logger.error(
                    "file-service content HTTP %d for doc_id=%s media_id=%s",
                    content_resp.code,
                    doc_id,
                    media_id,
                )
                return None

            self._breaker.on_success()
            logger.debug(
                "Serving media_id=%s from file-service doc_id=%s", media_id, doc_id
            )
            return _FileServiceResponder(content_resp)

        except Exception as exc:  # noqa: BLE001
            self._breaker.on_failure()
            logger.error(
                "file-service fetch error for media_id=%s: %s", media_id, exc
            )
            return None


def _read_bytes(file_path: str) -> bytes:
    """Blocking file read, run in a threadpool via defer_to_thread."""
    with open(file_path, "rb") as fh:
        return fh.read()

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
                 externalReference = media_id (= file_info.file_id). A durable
                 store is confirmed ONLY by HTTP 201 Created (the file-service
                 create contract); anything else trips the write breaker and
                 fails loudly. The uploaded file is STREAMED from the local cache
                 straight into the multipart body (never fully buffered).
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

SOURCE OF TRUTH: THIS file
(`matrix-adapter/synapse-modules/alkemio_fileservice_provider.py`) is the
canonical copy of the module. It is propagated verbatim to the downstream
deployment repos by `.github/workflows/sync-synapse-module.yml` — the inline
`data:` block embedded in each `01-synapse-setup-confmap.yml` (dev-orchestration,
infrastructure-operations) and the file copy in `server` are GENERATED from this
file by that workflow. Never hand-edit the downstream confmap copies; change this
file and let the sync workflow roll it forward.
"""

import logging
import math
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

# The file-service create contract: a durable store is confirmed by 201 Created.
_STORE_SUCCESS_CODE = 201


class _CircuitOpenError(Exception):
    """Raised when the circuit breaker is open and is short-circuiting calls."""


# Circuit-breaker states.
_CLOSED = "closed"
_OPEN = "open"
_HALF_OPEN = "half_open"


class _CircuitBreaker:
    """
    In-memory circuit breaker for ONE logical dependency direction (read or write).

    State machine — all mutations happen in synchronous, non-awaiting methods, so
    interleaved coroutines on the single Twisted reactor thread never observe a
    torn state and NO lock is required:

        CLOSED     calls pass through; consecutive failures are counted. At
                   `fail_threshold` consecutive failures -> OPEN.
        OPEN       calls short-circuit (`_CircuitOpenError`) until `reset_timeout`
                   elapses; the NEXT `before_call()` then admits exactly ONE probe
                   and moves to HALF_OPEN.
        HALF_OPEN  a single probe is in flight; every OTHER `before_call()`
                   short-circuits (no thundering herd). Probe success -> CLOSED;
                   probe failure -> OPEN (timer restarted); probe neutral (a clean
                   404 miss — service demonstrably reachable) -> CLOSED.

    A NEUTRAL outcome in CLOSED leaves the failure count UNTOUCHED: a cache miss
    is neither a success (which would zero real accumulated failures and stop the
    breaker ever tripping on a service that returns misses between hard errors)
    nor a failure.

    Time source is injectable for deterministic tests.
    """

    def __init__(self, fail_threshold, reset_timeout, clock=None, name="file-service"):
        import time as _time

        self._fail_threshold = fail_threshold
        self._reset_timeout = reset_timeout
        self._clock = clock or _time.monotonic
        self._name = name
        self._state = _CLOSED
        self._failures = 0
        self._opened_at = None  # type: Optional[float]

    @property
    def state(self) -> str:
        return self._state

    def before_call(self) -> None:
        if self._state == _CLOSED:
            return
        if self._state == _HALF_OPEN:
            # A probe is already being trialled. The single-trial guarantee is
            # enforced purely by the state machine: OPEN -> HALF_OPEN happens
            # exactly once (below), and every subsequent before_call() in
            # HALF_OPEN short-circuits here until the probe resolves. No extra
            # flag needed — and safe without a lock because these methods never
            # await (see class docstring).
            raise _CircuitOpenError(
                "%s circuit is half-open (trial call in flight)" % self._name
            )
        # OPEN
        if (self._clock() - self._opened_at) >= self._reset_timeout:
            self._state = _HALF_OPEN
            logger.info(
                "%s circuit half-open: admitting a single trial call", self._name
            )
            return
        raise _CircuitOpenError("%s circuit is open" % self._name)

    def on_success(self) -> None:
        if self._state != _CLOSED or self._failures:
            logger.info("%s circuit closed after success", self._name)
        self._reset()

    def on_failure(self) -> None:
        if self._state == _HALF_OPEN:
            # Recovery probe failed: straight back to OPEN, restart the timer so
            # the next probe only fires after another full reset window.
            self._state = _OPEN
            self._opened_at = self._clock()
            logger.warning("%s circuit re-OPEN: trial call failed", self._name)
            return
        if self._state == _OPEN:
            # Calls are short-circuited while OPEN, so this is not expected; keep
            # the timer as-is rather than continually pushing it out.
            return
        self._failures += 1
        if self._failures >= self._fail_threshold:
            self._state = _OPEN
            self._opened_at = self._clock()
            logger.warning(
                "%s circuit OPEN after %d consecutive failures",
                self._name,
                self._failures,
            )

    def on_neutral(self) -> None:
        """Record an outcome that is neither success nor failure (e.g. a 404 miss)."""
        if self._state == _HALF_OPEN:
            # The probe reached the service and got a valid HTTP response (the
            # service IS up; the doc is simply absent). Count the probe as passed
            # so recovery is not stalled waiting for a cache HIT.
            logger.info(
                "%s circuit closed: trial call returned a clean miss", self._name
            )
            self._reset()
        # In CLOSED we deliberately leave the failure count untouched.

    def _reset(self) -> None:
        self._state = _CLOSED
        self._failures = 0
        self._opened_at = None


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
        # ResponseDone arrives here as a clean close; treat any clean close as
        # "done" because the consumer has already received everything streamed.
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


class _DrainAndAbort(Protocol):
    """
    Body protocol used to RELEASE an unconsumed response body: it aborts the
    transport as soon as the body is delivered, so the (unbuffered) connection is
    torn down / returned to the pool instead of leaking a half-open socket.
    """

    def makeConnection(self, transport) -> None:
        Protocol.makeConnection(self, transport)
        # Abort the underlying connection: the response-body transport is an
        # IProducer, and stopProducing() tears the connection down.
        try:
            transport.stopProducing()
        except Exception:  # noqa: BLE001 - best-effort release
            pass

    def dataReceived(self, data: bytes) -> None:  # pragma: no cover - aborted
        pass


class _FileServiceResponder(Responder):
    """
    Streams a file-service content response into the media consumer without
    buffering the whole blob in memory.

    The responder deliberately does NOT touch the circuit breaker. The read
    breaker outcome is decided entirely in `fetch`, at RESPONSE-RECEIVED time
    (200 -> success): a breaker probe must resolve on "is file-service reachable
    and responding", which a slow/large body stream (minutes) must not gate. A
    mid-stream body drop is a network/client event, not a file-service-health
    signal, so it correctly does not feed the breaker.

    Connection lifecycle (finding 2): if Synapse enters the `with` block but never
    calls `write_to_consumer` (client disconnect / exception before streaming),
    `__exit__` ABORTS the unbuffered connection so the treq pool is not exhausted.
    """

    def __init__(self, response):
        self._response = response
        self._streamed = False
        self._aborted = False

    def write_to_consumer(self, consumer: IConsumer) -> "Deferred[int]":
        self._streamed = True
        finished = defer.Deferred()  # type: Deferred[int]
        try:
            self._response.deliverBody(_ConsumerSink(consumer, finished))
        except Exception as exc:  # noqa: BLE001 - a synchronous deliverBody raise
            # ...must not hang the caller waiting on `finished`; surface it as an
            # errback instead (finding 8).
            if not finished.called:
                finished.errback(exc)
        return make_deferred_yieldable(finished)

    def _abort(self) -> None:
        if self._aborted:
            return
        self._aborted = True
        try:
            self._response.deliverBody(_DrainAndAbort())
        except Exception as exc:  # noqa: BLE001 - best-effort release
            logger.debug("responder: best-effort connection abort failed: %s", exc)

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        if not self._streamed:
            # Body was never consumed (client disconnect / exception). Release the
            # unbuffered connection so the treq pool is not exhausted.
            self._abort()
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
            "timeout_s": _positive_number(
                config, "timeout_s", DEFAULT_TIMEOUT_S, float
            ),
            "store_timeout_s": _positive_number(
                config, "store_timeout_s", DEFAULT_STORE_TIMEOUT_S, float
            ),
            "cb_fail_threshold": _positive_number(
                config, "cb_fail_threshold", DEFAULT_CB_FAIL_THRESHOLD, int
            ),
            "cb_reset_timeout_s": _positive_number(
                config, "cb_reset_timeout_s", DEFAULT_CB_RESET_TIMEOUT_S, float
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
        # Separate breakers per direction: a store (write) outage MUST NOT open
        # the breaker that guards fetch (read). Element media reads keep working
        # while the write endpoint is down, and vice versa.
        self._read_breaker = _CircuitBreaker(
            config["cb_fail_threshold"],
            config["cb_reset_timeout_s"],
            name="file-service read",
        )
        self._write_breaker = _CircuitBreaker(
            config["cb_fail_threshold"],
            config["cb_reset_timeout_s"],
            name="file-service write",
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

    @staticmethod
    async def _drain_quietly(resp) -> None:
        """
        Drain a response body best-effort, SWALLOWING any error.

        Used for the small bodies of miss/error responses (and the already-durable
        201) so the connection can be reused. Its failure must never propagate:
        a drain error must not invert a neutral 404 into a breaker failure
        (finding 3) nor turn a durable 201 store into a reported failure
        (finding 4).
        """
        try:
            await make_deferred_yieldable(resp.content())
        except Exception as exc:  # noqa: BLE001 - best-effort drain
            logger.debug("file-service response drain failed (ignored): %s", exc)

    # -- StorageProvider API ------------------------------------------------

    async def store_file(self, path: str, file_info: "FileInfo") -> None:
        """Offload a freshly-uploaded local file to file-service, verbatim."""
        if not self._is_user_upload(file_info):
            # Thumbnail / url-cache / remote: leave in local cache only.
            return

        media_id = file_info.file_id

        try:
            self._write_breaker.before_call()
        except _CircuitOpenError:
            # store_synchronous=true: surface so the upload fails loudly rather
            # than silently dropping the only durable copy.
            logger.error(
                "file-service write circuit open; refusing to store media_id=%s",
                media_id,
            )
            raise

        # Everything after before_call() is wrapped so the write breaker records
        # EXACTLY ONE outcome on every exit path (a HALF_OPEN probe must never be
        # able to leave the breaker un-resolved / wedged — e.g. if the open below
        # raises). `recorded` guards against double-recording.
        url = "%s/internal/file" % self.file_service_url
        stream = None
        recorded = False
        try:
            # The freshly-uploaded file lives in the local media cache at
            # media_store_path + `path` (the relative path Synapse hands us, e.g.
            # `local_content/aa/bb/<rest>`). Synapse's FileInfo has NO `upload_path`
            # attribute — derive the absolute path the same way the on-disk store
            # does (matching synapse-s3-storage-provider). Opening it is blocking
            # I/O — do it off the reactor; treq then STREAMS the handle into the
            # multipart body via twisted's cooperative FileBodyProducer (chunked
            # 64 KiB reads scheduled on a Cooperator), so the file is never fully
            # copied into memory nor read synchronously on the reactor thread.
            cache_file = os.path.join(self.cache_path, path)
            stream = await defer_to_thread(self.reactor, _open_stream, cache_file)

            # treq serialises the multipart body as form-fields (`data`) THEN
            # files, preserving dict insertion order — so storageBucketId /
            # externalReference / skipImageProcessing precede the file part, which
            # file-service requires (it reads the metadata fields before consuming
            # the streamed file).
            files = {"file": (media_id, stream)}
            data = {
                "storageBucketId": self.matrix_media_bucket_id,
                "externalReference": media_id,
                "skipImageProcessing": "true",  # VERBATIM — read-back is exact
            }

            resp = await make_deferred_yieldable(
                treq.post(
                    url,
                    files=files,
                    data=data,
                    timeout=self.store_timeout_s,
                    reactor=self.reactor,
                )
            )

            if resp.code != _STORE_SUCCESS_CODE:
                # Only 201 Created confirms a durable store. A 2xx-non-201, a 3xx
                # redirect, or a 4xx/5xx is NOT a confirmed store — trip the write
                # breaker and fail loudly (store_synchronous surfaces it).
                self._write_breaker.on_failure()
                recorded = True
                await self._drain_quietly(resp)
                raise RuntimeError(
                    "file-service store returned HTTP %d (expected %d) for media_id=%s"
                    % (resp.code, _STORE_SUCCESS_CODE, media_id)
                )

            # 201: the media is durably stored. From here the store has SUCCEEDED
            # — a failure while draining the (already-consumed) response body must
            # NOT turn a durable store into a reported failure under
            # store_synchronous=true (finding 4).
            self._write_breaker.on_success()
            recorded = True
            await self._drain_quietly(resp)  # best-effort; conn reuse only
            logger.debug(
                "Stored media_id=%s in file-service bucket=%s",
                media_id,
                self.matrix_media_bucket_id,
            )
        except Exception as exc:  # noqa: BLE001 - convert any transport/IO error
            if not recorded:
                # Open failed, transport failed, or any other error before a
                # definitive outcome: record the single failure here.
                self._write_breaker.on_failure()
                logger.error(
                    "file-service store failed for media_id=%s: %s", media_id, exc
                )
            raise
        finally:
            if stream is not None:
                try:
                    stream.close()
                except Exception:  # noqa: BLE001 - best-effort
                    pass

    async def fetch(self, path: str, file_info: "FileInfo") -> Optional[Responder]:
        """
        Serve a media byte stream from file-service on local cache miss.

        Looks the document up GLOBALLY by externalReference (= media_id): the
        server may have MOVED it out of the staging bucket into a conversation
        bucket, so a bucket-scoped lookup would miss.
        """
        media_id = file_info.file_id

        try:
            self._read_breaker.before_call()
        except _CircuitOpenError:
            logger.warning(
                "file-service read circuit open; fetch miss for media_id=%s",
                media_id,
            )
            return None

        lookup_url = "%s/internal/file/by-reference?ref=%s" % (
            self.file_service_url,
            media_id,
        )

        # The read breaker records EXACTLY ONE outcome per fetch, decided here
        # around the request/response — NEVER deferred to body-stream completion
        # (a large/slow stream must not gate a reachability probe). Contract:
        #   - success  : a usable response was received (content GET -> 200).
        #   - neutral  : a clean miss (by-reference 404, or content 404 from a
        #                doc removed mid-fetch) — leaves the failure count intact.
        #   - failure  : no usable response (exception/timeout/>=500, malformed
        #                body, missing id).
        # 404 branches drain their small body via `_drain_quietly` so a drain
        # error can NEVER reach the outer `except` and invert neutral into a
        # failure (finding 3). Each hard-error branch records on_failure exactly
        # once and returns, so the outer `except` only fires for errors that
        # bypassed the explicit branches.
        try:
            meta_resp = await make_deferred_yieldable(
                treq.get(lookup_url, timeout=self.timeout_s, reactor=self.reactor)
            )
            if meta_resp.code == 404:
                self._read_breaker.on_neutral()
                await self._drain_quietly(meta_resp)
                return None
            if meta_resp.code >= 400:
                self._read_breaker.on_failure()
                await self._drain_quietly(meta_resp)
                logger.error(
                    "file-service by-reference HTTP %d for media_id=%s",
                    meta_resp.code,
                    media_id,
                )
                return None

            meta = await make_deferred_yieldable(treq.json_content(meta_resp))
            if not isinstance(meta, dict):
                # A null / non-object JSON body is a protocol error, not a miss:
                # count it as a failure rather than swallowing an AttributeError.
                self._read_breaker.on_failure()
                logger.error(
                    "file-service by-reference returned a non-object body for "
                    "media_id=%s: %r",
                    media_id,
                    type(meta).__name__,
                )
                return None
            doc_id = meta.get("id")
            if not doc_id:
                self._read_breaker.on_failure()
                logger.error(
                    "file-service by-reference returned no id for media_id=%s",
                    media_id,
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
            if content_resp.code == 404:
                # The doc was deleted between the by-reference lookup and the
                # content GET: a race, not a fault. Treat as a neutral miss.
                self._read_breaker.on_neutral()
                await self._drain_quietly(content_resp)
                logger.info(
                    "file-service content 404 (doc removed mid-fetch) for "
                    "doc_id=%s media_id=%s",
                    doc_id,
                    media_id,
                )
                return None
            if content_resp.code >= 400:
                self._read_breaker.on_failure()
                await self._drain_quietly(content_resp)
                logger.error(
                    "file-service content HTTP %d for doc_id=%s media_id=%s",
                    content_resp.code,
                    doc_id,
                    media_id,
                )
                return None

            # 200: the response is obtained, so file-service is reachable and
            # responding — record success NOW (resolves a HALF_OPEN probe the
            # instant the response arrives, independent of how long the body
            # takes to stream). The responder does not touch the breaker.
            self._read_breaker.on_success()
            logger.debug(
                "Serving media_id=%s from file-service doc_id=%s", media_id, doc_id
            )
            return _FileServiceResponder(content_resp)

        except Exception as exc:  # noqa: BLE001
            self._read_breaker.on_failure()
            logger.error(
                "file-service fetch error for media_id=%s: %s", media_id, exc
            )
            return None


def _positive_number(config: dict, key: str, default, cast):
    """
    Coerce a numeric config value, falling back to `default` when the key is
    absent OR present-but-null (YAML `key:` with no value yields None, which
    would otherwise blow up `float(None)`/`int(None)`). Rejects values that are
    not a finite positive number:
      - bool is an int subclass, so a YAML `true`/`false` would otherwise pass as
        1/0 — reject it explicitly (finding 6);
      - NaN slips past `<= 0` (all NaN comparisons are False) and +inf passes
        `> 0`, so require `math.isfinite` (finding 5);
      - a 0 fail-threshold would open the breaker immediately and a 0/negative
        timeout is nonsensical — require strictly positive.
    """
    raw = config.get(key)
    if raw is None:
        raw = default
    if isinstance(raw, bool):
        raise ValueError(
            "FileServiceStorageProvider: '%s' must be a number, not a bool" % key
        )
    try:
        value = cast(raw)
    except (TypeError, ValueError, OverflowError):
        raise ValueError(
            "FileServiceStorageProvider: '%s' must be a number, got %r" % (key, raw)
        )
    if not math.isfinite(value):
        raise ValueError(
            "FileServiceStorageProvider: '%s' must be finite, got %r" % (key, value)
        )
    if value <= 0:
        raise ValueError(
            "FileServiceStorageProvider: '%s' must be > 0, got %r" % (key, value)
        )
    return value


def _open_stream(file_path: str):
    """Open the local cache file for streaming into the multipart body.

    Called via defer_to_thread so the open() syscall never runs on the reactor;
    treq's MultiPartProducer then reads the handle in cooperative chunks, so the
    file is streamed rather than buffered whole in memory.
    """
    return open(file_path, "rb")

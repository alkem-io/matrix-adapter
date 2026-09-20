# Copyright 2026 Alkemio Foundation
# SPDX-License-Identifier: EUPL-1.2

"""
Pytest bootstrap for the Synapse module tests.

`alkemio_fileservice_provider` imports three Synapse symbols at MODULE level, so
without Synapse installed the test suite cannot even be COLLECTED. Synapse is a
heavy dependency (and not pip-installable in a plain CI job), which is why the
suite historically ran nowhere and gated nothing.

This file makes the suite hermetic: if `synapse` is genuinely importable (e.g.
running inside the matrixdotorg/synapse image) NOTHING here applies and the real
package is used. Only when it is absent do we register minimal `sys.modules`
stand-ins for exactly the symbols imported at module load:

    synapse.logging.context.defer_to_thread
    synapse.logging.context.make_deferred_yieldable
    synapse.media._base.Responder
    synapse.media.storage_provider.StorageProvider

These are IMPORT-TIME stand-ins. Every test that exercises behaviour through
them monkeypatches them anyway (see the `_patch_async_helpers` autouse fixture),
so the shims only need the right shape. The production import path is untouched.

It also puts this directory on sys.path so `import alkemio_fileservice_provider`
resolves no matter where pytest is invoked from.
"""

import os
import sys
import types

_HERE = os.path.dirname(os.path.abspath(__file__))
if _HERE not in sys.path:
    sys.path.insert(0, _HERE)


def _install_synapse_shims() -> None:
    """Register minimal synapse.* modules so the provider imports on a bare host."""
    from twisted.internet import defer, threads

    def make_deferred_yieldable(deferred):
        """Stand-in for Synapse's logcontext-preserving await helper.

        Synapse's real version detaches/reattaches the logging context around an
        await. There is no logcontext here, so pass the Deferred through.
        """
        return deferred

    def defer_to_thread(reactor, f, *args, **kwargs):
        """Stand-in for Synapse's logcontext-preserving deferToThread.

        Uses twisted's own threadpool bridge when the reactor provides one;
        otherwise (e.g. a `twisted.internet.task.Clock`) it runs `f` inline and
        wraps the outcome, which is all the import-time shape requires.
        """
        get_pool = getattr(reactor, "getThreadPool", None)
        if get_pool is None:
            return defer.execute(f, *args, **kwargs)
        return threads.deferToThreadPool(reactor, get_pool(), f, *args, **kwargs)

    class Responder:
        """Stand-in for synapse.media._base.Responder (a context manager)."""

        def write_to_consumer(self, consumer):
            raise NotImplementedError

        def __enter__(self):
            return self

        def __exit__(self, exc_type, exc_val, exc_tb):
            return None

    class StorageProvider:
        """Stand-in for synapse.media.storage_provider.StorageProvider."""

        async def store_file(self, path, file_info):
            raise NotImplementedError

        async def fetch(self, path, file_info):
            raise NotImplementedError

    def _module(name, **attrs):
        mod = types.ModuleType(name)
        for key, value in attrs.items():
            setattr(mod, key, value)
        sys.modules[name] = mod
        return mod

    synapse = _module("synapse")
    logging_pkg = _module("synapse.logging")
    context = _module(
        "synapse.logging.context",
        defer_to_thread=defer_to_thread,
        make_deferred_yieldable=make_deferred_yieldable,
    )
    media = _module("synapse.media")
    base = _module("synapse.media._base", Responder=Responder)
    storage_provider = _module(
        "synapse.media.storage_provider", StorageProvider=StorageProvider
    )

    # Wire the attribute tree so `from synapse.media._base import Responder`
    # works with either import machinery path.
    synapse.logging = logging_pkg
    synapse.media = media
    logging_pkg.context = context
    media._base = base
    media.storage_provider = storage_provider


try:  # pragma: no cover - depends on the host, exercised by both branches in CI/Synapse
    import synapse  # noqa: F401
except ImportError:  # pragma: no cover - see above
    _install_synapse_shims()

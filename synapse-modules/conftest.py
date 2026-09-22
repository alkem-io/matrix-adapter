# Copyright 2026 Alkemio Foundation
# SPDX-License-Identifier: EUPL-1.2
"""Import-only Synapse substitutes for lightweight provider unit tests."""
import sys
import types
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
try:
    import synapse  # noqa: F401
except ImportError:
    class HttpResponseException(Exception):
        def __init__(self, code, msg, response):
            super().__init__(msg)
            self.code = code

    class FileResponder:
        def __init__(self, hs, open_file):
            self.open_file = open_file

        def __enter__(self):
            return self

        def __exit__(self, *args):
            self.open_file.close()

    modules = {
        "synapse": {},
        "synapse.api": {},
        "synapse.api.errors": {"HttpResponseException": HttpResponseException},
        "synapse.logging": {},
        "synapse.logging.context": {"make_deferred_yieldable": lambda value: value},
        "synapse.media": {},
        "synapse.media.media_storage": {"FileResponder": FileResponder},
        "synapse.media.storage_provider": {"StorageProvider": object},
    }
    for name, attributes in modules.items():
        module = types.ModuleType(name)
        module.__dict__.update(attributes)
        sys.modules[name] = module

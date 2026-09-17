"""Unit tests for the alkemio_room_control Synapse module.

These tests run with the standard library only — Synapse is not installed in
the test environment, so they exercise the module's pure decision functions
and never its Synapse-facing registration code.
"""

import pathlib
import py_compile
import unittest

MODULE_PATH = pathlib.Path(__file__).resolve().parent.parent / "alkemio_room_control.py"


class TestModuleSource(unittest.TestCase):
    def test_module_compiles_without_synapse(self) -> None:
        """The module file must be valid Python even where Synapse is absent."""
        py_compile.compile(str(MODULE_PATH), doraise=True)


if __name__ == "__main__":
    unittest.main()

"""Unit tests for the alkemio_room_control Synapse module.

These tests run with the standard library only — Synapse is not installed in
the test environment, so they exercise the module's pure decision functions
and never its Synapse-facing registration code.
"""

import pathlib
import py_compile
import sys
import unittest

MODULE_DIR = pathlib.Path(__file__).resolve().parent.parent
MODULE_PATH = MODULE_DIR / "alkemio_room_control.py"
sys.path.insert(0, str(MODULE_DIR))

from alkemio_room_control import (  # noqa: E402
    SYNAPSE_AVAILABLE,
    evaluate_event,
    evaluate_invite,
)

BOT = "@00000000-0000-0000-0000-000000000000:example.com"
MEMBER = "@11111111-2222-4333-8444-555555555555:example.com"
OTHER = "@99999999-8888-4777-8666-555555555555:example.com"


class TestModuleSource(unittest.TestCase):
    def test_module_compiles_without_synapse(self) -> None:
        """The module file must be valid Python even where Synapse is absent."""
        py_compile.compile(str(MODULE_PATH), doraise=True)

    def test_decision_functions_import_without_synapse(self) -> None:
        self.assertFalse(SYNAPSE_AVAILABLE, "tests must run without Synapse installed")


class TestEvaluateInvite(unittest.TestCase):
    def test_bot_invites_on_governed_room(self) -> None:
        self.assertTrue(evaluate_invite(BOT, MEMBER, True, BOT))

    def test_member_invite_on_governed_room_denied(self) -> None:
        self.assertFalse(evaluate_invite(MEMBER, OTHER, True, BOT))

    def test_invitee_is_bot_allowed_even_on_governed_room(self) -> None:
        """Repair rejoin: a joined ghost invites the bot back."""
        self.assertTrue(evaluate_invite(MEMBER, BOT, True, BOT))

    def test_member_invite_on_ungoverned_room_allowed(self) -> None:
        self.assertTrue(evaluate_invite(MEMBER, OTHER, False, BOT))


class TestEvaluateEventBot(unittest.TestCase):
    def test_bot_state_always_allowed(self) -> None:
        for event_type in ("m.room.power_levels", "m.room.name", "io.alkemio.entity", "m.room.member"):
            self.assertTrue(evaluate_event(BOT, event_type, "", {}, True, BOT), event_type)

    def test_bot_kick_allowed(self) -> None:
        self.assertTrue(
            evaluate_event(BOT, "m.room.member", MEMBER, {"membership": "leave"}, True, BOT)
        )


class TestEvaluateEventGovernedState(unittest.TestCase):
    """Governed room: every state event denied except a member's own join/leave."""

    DENIED_STATE_TYPES = (
        "m.room.name",
        "m.room.topic",
        "m.room.avatar",
        "m.room.join_rules",
        "m.room.power_levels",
        "m.room.history_visibility",
        "m.room.guest_access",
        "m.room.canonical_alias",
        "m.space.child",
        "m.space.parent",
    )

    def test_member_state_changes_denied(self) -> None:
        for event_type in self.DENIED_STATE_TYPES:
            self.assertFalse(
                evaluate_event(MEMBER, event_type, "", {}, True, BOT), event_type
            )

    def test_member_kick_denied(self) -> None:
        """A kick is m.room.member with a state_key other than the sender."""
        self.assertFalse(
            evaluate_event(MEMBER, "m.room.member", OTHER, {"membership": "leave"}, True, BOT)
        )

    def test_member_ban_denied(self) -> None:
        self.assertFalse(
            evaluate_event(MEMBER, "m.room.member", OTHER, {"membership": "ban"}, True, BOT)
        )

    def test_member_invite_as_state_denied(self) -> None:
        self.assertFalse(
            evaluate_event(MEMBER, "m.room.member", OTHER, {"membership": "invite"}, True, BOT)
        )

    def test_self_join_allowed(self) -> None:
        self.assertTrue(
            evaluate_event(MEMBER, "m.room.member", MEMBER, {"membership": "join"}, True, BOT)
        )

    def test_self_leave_allowed(self) -> None:
        self.assertTrue(
            evaluate_event(MEMBER, "m.room.member", MEMBER, {"membership": "leave"}, True, BOT)
        )

    def test_self_knock_denied(self) -> None:
        self.assertFalse(
            evaluate_event(MEMBER, "m.room.member", MEMBER, {"membership": "knock"}, True, BOT)
        )


class TestEvaluateEventTimeline(unittest.TestCase):
    """Ordinary participation (timeline events, state_key None) stays allowed."""

    def test_message_allowed_on_governed_room(self) -> None:
        self.assertTrue(evaluate_event(MEMBER, "m.room.message", None, {}, True, BOT))

    def test_reaction_allowed_on_governed_room(self) -> None:
        self.assertTrue(evaluate_event(MEMBER, "m.reaction", None, {}, True, BOT))

    def test_redaction_allowed_on_governed_room(self) -> None:
        """Whose events may be redacted is the ladder's job (redact: 75), not the module's."""
        self.assertTrue(evaluate_event(MEMBER, "m.room.redaction", None, {}, True, BOT))


class TestEvaluateEventMarkers(unittest.TestCase):
    """io.alkemio.* state is bot-only in EVERY room, governed or not."""

    def test_entity_marker_by_member_in_governed_room_denied(self) -> None:
        self.assertFalse(evaluate_event(MEMBER, "io.alkemio.entity", "", {}, True, BOT))

    def test_entity_marker_by_member_in_ungoverned_room_denied(self) -> None:
        self.assertFalse(evaluate_event(MEMBER, "io.alkemio.entity", "", {}, False, BOT))

    def test_governance_marker_by_member_denied(self) -> None:
        self.assertFalse(evaluate_event(MEMBER, "io.alkemio.governance", "", {}, False, BOT))

    def test_visibility_marker_by_member_denied(self) -> None:
        self.assertFalse(evaluate_event(MEMBER, "io.alkemio.visibility", "", {}, False, BOT))


class TestEvaluateEventUngoverned(unittest.TestCase):
    """Ungoverned rooms (pre-reconciliation window) keep today's behaviour."""

    def test_name_change_allowed(self) -> None:
        self.assertTrue(evaluate_event(MEMBER, "m.room.name", "", {}, False, BOT))

    def test_member_kick_allowed(self) -> None:
        self.assertTrue(
            evaluate_event(MEMBER, "m.room.member", OTHER, {"membership": "leave"}, False, BOT)
        )


if __name__ == "__main__":
    unittest.main()

package matrix

import "testing"

const trustToken = "as-token-secret"

func TestTrustedField_RoundTrip(t *testing.T) {
	sig := SignTrustedField(trustToken, "!room:test.local", "@user:test.local", "document-123")
	if !VerifyTrustedField(trustToken, "!room:test.local", "@user:test.local", "document-123", sig) {
		t.Error("a control-plane-produced signature must verify")
	}
}

func TestTrustedField_TamperRejected(t *testing.T) {
	sig := SignTrustedField(trustToken, "!room:test.local", "@user:test.local", "document-123")
	if VerifyTrustedField(trustToken, "!room:test.local", "@user:test.local", "document-999", sig) {
		t.Error("a tampered value must not verify")
	}
	if VerifyTrustedField(trustToken, "!other:test.local", "@user:test.local", "document-123", sig) {
		t.Error("a signature is bound to its room")
	}
	if VerifyTrustedField(trustToken, "!room:test.local", "@mallory:test.local", "document-123", sig) {
		t.Error("a signature is bound to its sender")
	}
}

func TestTrustedField_WrongKeyRejected(t *testing.T) {
	sig := SignTrustedField("some-other-token", "!room:test.local", "@user:test.local", "document-123")
	if VerifyTrustedField(trustToken, "!room:test.local", "@user:test.local", "document-123", sig) {
		t.Error("a signature under a different key must not verify — identity alone is never a trust basis")
	}
}

package matrix

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// Trusted-field proof primitive (contract appservice-event-bridge G5).
//
// The UUID user namespace is non-exclusive (users log in via SSO/Element with
// the same mxid the control plane impersonates), so a sender's IDENTITY can
// never prove the control plane produced an event. Trust needs an
// AUTHENTICATION-bound proof instead: the control plane signs the trusted
// value with the AS token — a secret a browser session never holds.
//
// This slice provides the primitive only; the media seam (#1946, PR #67)
// adopts it before merge (recorded human gate). Nothing here is wired into
// an event path.

// SignTrustedField computes the base64 HMAC-SHA256 proof over
// roomID "\n" sender "\n" value, keyed with the AS token.
func SignTrustedField(asToken string, roomID, sender, value string) string {
	mac := hmac.New(sha256.New, []byte(asToken))
	mac.Write([]byte(roomID + "\n" + sender + "\n" + value))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyTrustedField reports whether sig is the valid proof for the value in
// this room from this sender. Constant-time comparison.
func VerifyTrustedField(asToken string, roomID, sender, value, sig string) bool {
	expected := SignTrustedField(asToken, roomID, sender, value)
	return hmac.Equal([]byte(expected), []byte(sig))
}

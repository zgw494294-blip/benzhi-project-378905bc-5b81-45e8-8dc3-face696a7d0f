package domain

import (
	"encoding/hex"
	"strings"
)

// NormalizeIdentifier keeps identifiers stable at API boundaries while
// preserving the original case used by callers for display.
func NormalizeIdentifier(value string) string {
	return strings.TrimSpace(value)
}

// IsHexDigest reports whether value is a complete SHA-256 digest.
func IsHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

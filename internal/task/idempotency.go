package task

import (
	"crypto/sha256"
	"encoding/hex"
)

// OperationDedup records one canonical operation submission so identical
// replays return the original outcome while different content conflicts.
type OperationDedup struct {
	TaskID       string
	OperationID  string
	Generation   int
	ContentHash  string
	ResponseJSON string
}

// NormalizeContent canonicalizes a JSON-ish command payload into a stable byte
// sequence so that field order and insignificant whitespace never change the
// deduplication key. It is deliberately conservative: the caller supplies an
// already JSON-marshalled canonical form.
func NormalizeContent(canonicalJSON []byte) string {
	sum := sha256.Sum256(canonicalJSON)
	return hex.EncodeToString(sum[:])
}

package arbitration

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// CredentialIssuer mints the unique, immutable incubation credential number
// issued only when a task is admitted. The number is deterministic over the
// task id and terminal version so it never changes and cannot collide across
// distinct tasks.
type CredentialIssuer struct{}

// Issue derives the credential number from the task id and final version.
func (CredentialIssuer) Issue(taskID string, version int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", taskID, version)))
	return "INC-" + hex.EncodeToString(sum[:8])
}

// Verify re-derives the number to confirm an issued credential still matches.
func (CredentialIssuer) Verify(taskID string, version int64, number string) bool {
	return CredentialIssuer{}.Issue(taskID, version) == number
}

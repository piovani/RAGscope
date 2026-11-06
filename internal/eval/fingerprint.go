package eval

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// Fingerprint is a short code that identifies the content of a golden set. It
// is saved with every report, so two results can be compared only when they
// were scored on the same questions. Comments and formatting in the YAML file
// do not change it; changing a question, an answer or an evidence snippet does.
func Fingerprint(questions []Question) string {
	data, _ := json.Marshal(questions) // plain structs, so this cannot fail
	return fmt.Sprintf("%x", sha256.Sum256(data))[:12]
}

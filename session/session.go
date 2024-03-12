// this is a sample session manager
package session

import (
	"crypto/rand"
	"encoding/base64"
)

func GenerateSessionId() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(b)
}

package main

import (
	"crypto/subtle"
	"encoding/base64"
	"strings"

	"github.com/datahearth/streamline/extensions/sdk"
)

// authenticate checks the request's Basic credentials against the user's
// stored token hash. ok is false for any credential problem; err is only for a
// failed lookup.
func authenticate(r sdk.Request) (userID int64, ok bool, err error) {
	name, pass, found := basicAuth(header(r, "Authorization"))
	if !found {
		return 0, false, nil
	}
	rows, err := query(
		`SELECT u.user_id, t.token_hash FROM streamline_user_logins u
		JOIN tokens t ON t.user_id = u.user_id
		JOIN streamline_users a ON a.id = u.user_id
		WHERE u.email = ? AND a.locked = 0`,
		strings.ToLower(name),
	)
	if err != nil || len(rows) == 0 {
		return 0, false, err
	}
	stored := asString(rows[0][1])
	if stored == "" ||
		subtle.ConstantTimeCompare([]byte(stored), []byte(hashToken(pass))) != 1 {
		return 0, false, nil
	}
	return asInt(rows[0][0]), true, nil
}

func basicAuth(h string) (user, pass string, ok bool) {
	scheme, enc, found := strings.Cut(h, " ")
	if !found || !strings.EqualFold(scheme, "Basic") {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(enc))
	if err != nil {
		return "", "", false
	}
	return strings.Cut(string(raw), ":")
}

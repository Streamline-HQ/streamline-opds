package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/datahearth/streamline/extensions/sdk"
)

const (
	touchInterval = 5 * time.Minute
	clientMaxLen  = 64
	storedTime    = "2006-01-02T15:04:05Z"
)

func query(sql string, args ...any) ([][]any, error) {
	res, err := sdk.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	if res.Error != "" {
		return nil, errors.New(res.Error)
	}
	return res.Rows, nil
}

func exec(sql string, args ...any) error {
	res, err := sdk.Exec(sql, args...)
	if err != nil {
		return err
	}
	if res.Error != "" {
		return errors.New(res.Error)
	}
	return nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asInt(v any) int64 {
	f, _ := v.(float64)
	return int64(f)
}

func nullable(v any) *string {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	return &s
}

// ensureSchema creates the token store and, once, copies the tokens core held
// before OPDS became an extension. The copy runs in one transaction that
// re-reads its own marker, so a token rotated or deleted afterwards is never
// brought back by a request that started before it.
func ensureSchema() error {
	if done, err := legacyCopied(); err == nil && done {
		return nil
	}
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS tokens(
			user_id INTEGER PRIMARY KEY,
			token_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_used_at TEXT,
			last_client TEXT)`,
		`CREATE TABLE IF NOT EXISTS meta(key TEXT PRIMARY KEY, value TEXT)`,
	} {
		if err := exec(ddl); err != nil {
			return err
		}
	}
	if err := exec("BEGIN IMMEDIATE"); err != nil {
		return err
	}
	if err := copyLegacy(); err != nil {
		_ = exec("ROLLBACK")
		return err
	}
	err := exec("COMMIT")
	return err
}

func legacyCopied() (bool, error) {
	rows, err := query("SELECT 1 FROM meta WHERE key = 'legacy_copied'")
	return len(rows) > 0, err
}

func copyLegacy() error {
	if done, err := legacyCopied(); err != nil || done {
		return err
	}
	rows, err := query(
		`SELECT user_id, token_hash, created_at, last_used_at, last_client
		FROM streamline_legacy_opds_tokens`,
	)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(storedTime)
	for _, r := range rows {
		created := normalizeTime(asString(r[2]), now)
		var lastUsed any
		if s := asString(r[3]); s != "" {
			lastUsed = normalizeTime(s, "")
		}
		if err := exec(
			`INSERT OR IGNORE INTO tokens(user_id, token_hash, created_at, last_used_at, last_client)
			VALUES(?, ?, ?, ?, ?)`,
			asInt(r[0]),
			asString(r[1]),
			created,
			lastUsed,
			nullable(r[4]),
		); err != nil {
			return err
		}
	}
	err = exec(
		"INSERT OR IGNORE INTO meta(key, value) VALUES('legacy_copied', '1')",
	)
	return err
}

func normalizeTime(s, fallback string) string {
	if t := parseTime(s); !t.IsZero() {
		return t.UTC().Format(storedTime)
	}
	return fallback
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func clientFromUserAgent(ua string) string {
	product, _, _ := strings.Cut(strings.TrimSpace(ua), " ")
	name, _, _ := strings.Cut(product, "/")
	return sanitizeClient(name)
}

func sanitizeClient(s string) string {
	s = strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	for len(s) > clientMaxLen {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}

func touch(userID int64, userAgent string) {
	now := time.Now().UTC()
	err := exec(
		`UPDATE tokens SET last_used_at = ?, last_client = ?
		WHERE user_id = ? AND (last_used_at IS NULL OR last_used_at < ?)`,
		now.Format(storedTime),
		clientFromUserAgent(userAgent),
		userID,
		now.Add(-touchInterval).Format(storedTime),
	)
	if err != nil {
		sdk.Log("warn", "stamping OPDS token use failed", "error", err.Error())
	}
}

func parseTime(s string) time.Time {
	if i := strings.Index(s, " m="); i >= 0 {
		s = s[:i]
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func errorf(format string, a ...any) error { return fmt.Errorf(format, a...) }

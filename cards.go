package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/datahearth/streamline/extensions/sdk"
)

func cards(r sdk.Request) sdk.Response {
	if r.Path != "/cards/token" && r.Path != "/cards/token/" {
		return notFound()
	}
	if r.Caller == nil {
		return text(http.StatusUnauthorized, "unauthorized")
	}
	var (
		resp sdk.Response
		err  error
	)
	switch strings.ToUpper(r.Method) {
	case http.MethodGet:
		resp, err = tokenStatus(int64(r.Caller.ID))
	case http.MethodPost:
		resp, err = rotateToken(int64(r.Caller.ID))
	case http.MethodDelete:
		resp, err = revokeToken(int64(r.Caller.ID))
	default:
		return text(http.StatusMethodNotAllowed, "method not allowed")
	}
	if err != nil {
		sdk.Log("error", "OPDS token card failed", "error", err.Error())
		return text(http.StatusInternalServerError, "internal error")
	}
	return resp
}

func jsonResponse(v any) (sdk.Response, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return sdk.Response{}, err
	}
	return sdk.Response{
		Status:  http.StatusOK,
		Headers: map[string][]string{"Content-Type": {"application/json"}},
		Body:    b,
	}, nil
}

func tokenStatus(userID int64) (sdk.Response, error) {
	rows, err := query(
		"SELECT created_at, last_used_at, last_client FROM tokens WHERE user_id = ?",
		userID,
	)
	if err != nil {
		return sdk.Response{}, err
	}
	out := struct {
		Set        bool    `json:"set"`
		CreatedAt  *string `json:"created_at"`
		LastUsedAt *string `json:"last_used_at"`
		LastClient *string `json:"last_client"`
	}{}
	if len(rows) > 0 {
		out.Set = true
		out.CreatedAt = nullable(rows[0][0])
		out.LastUsedAt = nullable(rows[0][1])
		out.LastClient = nullable(rows[0][2])
	}
	return jsonResponse(out)
}

func rotateToken(userID int64) (sdk.Response, error) {
	secret, err := newSecret()
	if err != nil {
		return sdk.Response{}, err
	}
	if err := exec(
		`INSERT INTO tokens(user_id, token_hash, created_at) VALUES(?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET token_hash = excluded.token_hash,
			created_at = excluded.created_at,
			last_used_at = NULL, last_client = NULL`,
		userID, hashToken(secret), time.Now().UTC().Format(storedTime),
	); err != nil {
		return sdk.Response{}, err
	}
	return jsonResponse(struct {
		Secret string `json:"secret"`
	}{secret})
}

func revokeToken(userID int64) (sdk.Response, error) {
	if err := exec("DELETE FROM tokens WHERE user_id = ?", userID); err != nil {
		return sdk.Response{}, err
	}
	return sdk.Response{Status: http.StatusNoContent}, nil
}

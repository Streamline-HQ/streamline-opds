// Command opds is Streamline's OPDS 1.2 catalog of the ebook library, run as
// an extension. Readers authenticate with HTTP Basic against a per-user token
// the extension keeps in its own database.
package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/datahearth/streamline/extensions/sdk"
)

func main() {}

//go:wasmexport manifest
func manifest() int32 {
	return sdk.WriteManifest(sdk.Manifest{
		Name:          "opds",
		Version:       "1.0.0",
		StreamlineAPI: 1,
		Mounts:        []sdk.Mount{{Prefix: "/opds", Auth: "self"}},
		Views: []string{
			"users", "user_logins", "books", "book_series", "authors",
			"book_contributions", "book_files",
		},
		Services: []string{"serve_file", "serve_poster"},
		AccountCards: []sdk.AccountCard{{
			ID:       "token",
			Kind:     "credential",
			Title:    "OPDS",
			Mount:    "/opds",
			Username: "email",
		}},
	})
}

//go:wasmexport handle
func handle() int32 { return sdk.Dispatch(route) }

func route(r sdk.Request) sdk.Response {
	if err := ensureSchema(); err != nil {
		sdk.Log("error", "preparing the token store failed", "error", err.Error())
		return text(http.StatusInternalServerError, "internal error")
	}
	if r.Path == "/cards" || strings.HasPrefix(r.Path, "/cards/") {
		return cards(r)
	}

	userID, ok, err := authenticate(r)
	if err != nil {
		sdk.Log("error", "OPDS authentication failed", "error", err.Error())
		return text(http.StatusInternalServerError, "internal error")
	}
	if !ok {
		return sdk.Response{
			Status: http.StatusUnauthorized,
			Headers: map[string][]string{
				"WWW-Authenticate": {`Basic realm="Streamline OPDS"`},
				"Content-Type":     {"text/plain; charset=utf-8"},
			},
			Body: []byte("unauthorized"),
		}
	}
	touch(userID, header(r, "User-Agent"))

	if r.Method != http.MethodGet {
		return text(http.StatusMethodNotAllowed, "method not allowed")
	}
	resp, err := catalog(r)
	if err != nil {
		sdk.Log("error", "OPDS request failed", "path", r.Path, "error", err.Error())
		return text(http.StatusInternalServerError, "internal error")
	}
	return resp
}

func catalog(r sdk.Request) (sdk.Response, error) {
	switch r.Path {
	case "", "/":
		return root(), nil
	case "/authors":
		return authors()
	case "/recent":
		return recent()
	case "/search.xml":
		return searchDescription(), nil
	case "/search":
		return search(r.Query)
	}
	if rest, ok := strings.CutPrefix(r.Path, "/authors/"); ok {
		id, ok := parseID(rest)
		if !ok {
			return notFound(), nil
		}
		return authorBooks(id)
	}
	if rest, ok := strings.CutPrefix(r.Path, "/cover/"); ok {
		id, ok := parseID(rest)
		if !ok {
			return notFound(), nil
		}
		return sdk.Response{
			ServePoster: &sdk.ServePoster{Kind: "books", ID: id},
		}, nil
	}
	if rest, ok := strings.CutPrefix(r.Path, "/download/"); ok {
		idPart, format, found := strings.Cut(rest, "/")
		id, ok := parseID(idPart)
		if !found || !ok || format == "" || strings.Contains(format, "/") {
			return notFound(), nil
		}
		return download(id, format)
	}
	return notFound(), nil
}

func parseID(s string) (uint32, bool) {
	n, err := strconv.ParseUint(s, 10, 32)
	return uint32(n), err == nil
}

func header(r sdk.Request, name string) string {
	for k, vs := range r.Headers {
		if strings.EqualFold(k, name) && len(vs) > 0 {
			return vs[0]
		}
	}
	return ""
}

func text(status int, msg string) sdk.Response {
	return sdk.Response{
		Status:  status,
		Headers: map[string][]string{"Content-Type": {"text/plain; charset=utf-8"}},
		Body:    []byte(msg + "\n"),
	}
}

func notFound() sdk.Response { return text(http.StatusNotFound, "404 page not found") }

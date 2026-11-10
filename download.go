package main

import (
	"path"
	"regexp"
	"strings"

	"github.com/datahearth/streamline/extensions/sdk"
)

var (
	pathReplacer = strings.NewReplacer(
		":", " -", "/", "-", "\\", "-", "<", "", ">", "", "\"", "", "|", "",
		"?", "", "*", "",
	)
	pathSpaceRun = regexp.MustCompile(`\s{2,}`)
)

func sanitizeName(s string) string {
	s = pathReplacer.Replace(s)
	return strings.TrimSpace(pathSpaceRun.ReplaceAllString(s, " "))
}

func download(bookID uint32, format string) (sdk.Response, error) {
	rows, err := query(
		`SELECT f.id, f.format, f.filename, b.title, b.author_name
		FROM streamline_book_files f
		JOIN streamline_books b ON b.id = f.book_id
		WHERE f.book_id = ? AND f.slot = 'ebook' AND lower(f.format) = lower(?)
		ORDER BY f.id DESC LIMIT 1`,
		bookID, format,
	)
	if err != nil {
		return sdk.Response{}, err
	}
	if len(rows) == 0 {
		return notFound(), nil
	}
	r := rows[0]
	stored := asString(r[1])
	ext := strings.ToLower(stored)
	if ext == "" || ext == "other" {
		ext = strings.TrimPrefix(path.Ext(asString(r[2])), ".")
	}
	name := asString(r[3]) + " - " + asString(r[4])
	if ext != "" {
		name += "." + ext
	}
	fileID := uint32(asInt(r[0])) //nolint:gosec // a book_files id, a uint32 column
	return sdk.Response{ServeFile: &sdk.ServeFile{
		FileID:      fileID,
		Filename:    sanitizeName(name),
		ContentType: ebookContentType(stored),
	}}, nil
}

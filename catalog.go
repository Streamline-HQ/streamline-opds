package main

import (
	"cmp"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/datahearth/streamline/extensions/sdk"
)

const recentLimit = 50

const openSearchDescription = `<?xml version="1.0" encoding="UTF-8"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>Streamline</ShortName>
  <Description>Search the Streamline ebook catalog</Description>
  <Url type="` + acquisitionType + `" template="/opds/search?q={searchTerms}"/>
</OpenSearchDescription>
`

// ebookLadder is the format ranking a profile uses, best first.
var ebookLadder = []string{"EPUB", "AZW3", "MOBI", "PDF", "CBZ", "CBR"}

// makers are the roles an entry credits; creators are the ones an author is
// listed under.
const (
	makerRoles   = `('author','writer','artist','colorist','cover')`
	creatorRoles = `('author','writer')`

	hasEbook = `EXISTS (SELECT 1 FROM streamline_book_files f
		WHERE f.book_id = b.id AND f.slot = 'ebook')`
)

func ebookContentType(format string) string {
	switch strings.ToUpper(format) {
	case "EPUB":
		return "application/epub+zip"
	case "PDF":
		return "application/pdf"
	case "MOBI":
		return "application/x-mobipocket-ebook"
	case "AZW3":
		return "application/x-mobi8-ebook"
	case "CBZ":
		return "application/vnd.comicbook+zip"
	case "CBR":
		return "application/vnd.comicbook-rar"
	default:
		return "application/octet-stream"
	}
}

func root() sdk.Response {
	resp, _ := render(kindNavigation, &feed{
		ID:    "urn:streamline:opds:root",
		Title: "Streamline",
		Links: []link{
			selfLink("/opds", kindNavigation),
			{Rel: "start", Href: "/opds", Type: navigationType},
			{Rel: "search", Href: "/opds/search.xml", Type: openSearchType},
		},
		Entries: []entry{
			{
				ID:    "urn:streamline:opds:authors",
				Title: "Authors",
				Links: []link{
					{Rel: "subsection", Href: "/opds/authors", Type: navigationType},
				},
			},
			{
				ID:    "urn:streamline:opds:recent",
				Title: "Recently added",
				Links: []link{{
					Rel:  "http://opds-spec.org/sort/new",
					Href: "/opds/recent",
					Type: acquisitionType,
				}},
			},
		},
	})
	return resp
}

func authors() (sdk.Response, error) {
	rows, err := query(`SELECT a.id, a.name, a.sort_name, MAX(b.updated_at)
		FROM streamline_authors a
		JOIN streamline_book_contributions c
			ON c.author_id = a.id AND c.role IN ` + creatorRoles + `
		JOIN streamline_books b ON b.id = c.book_id
		WHERE ` + hasEbook + `
		GROUP BY a.id`)
	if err != nil {
		return sdk.Response{}, err
	}
	slices.SortFunc(rows, func(a, b []any) int {
		return cmp.Compare(
			cmp.Or(asString(a[2]), asString(a[1])),
			cmp.Or(asString(b[2]), asString(b[1])),
		)
	})

	f := &feed{
		ID:    "urn:streamline:opds:authors",
		Title: "Authors",
		Links: []link{
			selfLink("/opds/authors", kindNavigation),
			{Rel: "start", Href: "/opds", Type: navigationType},
		},
	}
	for _, r := range rows {
		updated := parseTime(asString(r[3]))
		f.Entries = append(f.Entries, entry{
			ID:      fmt.Sprintf("urn:streamline:author:%d", asInt(r[0])),
			Title:   asString(r[1]),
			Updated: atomTime(updated),
			Links: []link{{
				Rel:  "subsection",
				Href: fmt.Sprintf("/opds/authors/%d", asInt(r[0])),
				Type: acquisitionType,
			}},
		})
		f.Updated = newest(f.Updated, updated)
	}
	return render(kindNavigation, f)
}

func authorBooks(id uint32) (sdk.Response, error) {
	rows, err := query("SELECT name FROM streamline_authors WHERE id = ?", id)
	if err != nil {
		return sdk.Response{}, err
	}
	if len(rows) == 0 {
		return notFound(), nil
	}
	books, err := findBooks(
		`b.id IN (SELECT c.book_id FROM streamline_book_contributions c
			WHERE c.author_id = ? AND c.role IN `+creatorRoles+`)`,
		[]any{id},
		"b.sort_title, b.title",
	)
	if err != nil {
		return sdk.Response{}, err
	}
	return acquisition(
		fmt.Sprintf("urn:streamline:author:%d", id),
		asString(rows[0][0]),
		fmt.Sprintf("/opds/authors/%d", id),
		books,
	)
}

func recent() (sdk.Response, error) {
	latest := func(agg string) string {
		return `(SELECT ` + agg + ` FROM streamline_book_files f
			WHERE f.book_id = b.id AND f.slot = 'ebook')`
	}
	books, err := findBooks(
		`b.id IN (SELECT book_id FROM streamline_book_files
			WHERE slot = 'ebook' AND book_id IS NOT NULL
			GROUP BY book_id
			ORDER BY MAX(added_at) DESC, MAX(id) DESC LIMIT ?)`,
		[]any{recentLimit},
		latest("MAX(f.added_at)")+" DESC, "+latest("MAX(f.id)")+" DESC",
	)
	if err != nil {
		return sdk.Response{}, err
	}
	return acquisition(
		"urn:streamline:opds:recent", "Recently added", "/opds/recent", books,
	)
}

func searchDescription() sdk.Response {
	return sdk.Response{
		Status:  200,
		Headers: map[string][]string{"Content-Type": {openSearchType}},
		Body:    []byte(openSearchDescription),
	}
}

func search(rawQuery string) (sdk.Response, error) {
	vals, _ := url.ParseQuery(rawQuery)
	q := vals.Get("q")
	if q == "" {
		return acquisition(
			"urn:streamline:opds:search", "Search", "/opds/search", nil,
		)
	}
	pattern := "%" + likeEscaper.Replace(q) + "%"
	books, err := findBooks(
		`(b.title LIKE ? ESCAPE '\' OR b.id IN (
			SELECT c.book_id FROM streamline_book_contributions c
			JOIN streamline_authors a ON a.id = c.author_id
			WHERE a.name LIKE ? ESCAPE '\'))`,
		[]any{pattern, pattern},
		"b.sort_title, b.title",
	)
	if err != nil {
		return sdk.Response{}, err
	}
	return acquisition(
		"urn:streamline:opds:search",
		"Search: "+q,
		"/opds/search?q="+url.QueryEscape(q),
		books,
	)
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

type bookFile struct {
	id     int64
	format string
}

type book struct {
	id       int64
	title    string
	overview string
	updated  time.Time
	authors  []string
	files    []bookFile
}

// findBooks returns the books holding a present ebook that match where, with
// the makers and ebook files each entry needs, in the order orderBy gives.
func findBooks(where string, args []any, orderBy string) ([]*book, error) {
	scope := hasEbook + " AND " + where
	rows, err := query(
		`SELECT b.id, b.title, b.overview, b.updated_at FROM streamline_books b
		WHERE `+scope+` ORDER BY `+orderBy,
		args...,
	)
	if err != nil {
		return nil, err
	}
	books := make([]*book, 0, len(rows))
	byID := make(map[int64]*book, len(rows))
	for _, r := range rows {
		bk := &book{
			id:       asInt(r[0]),
			title:    asString(r[1]),
			overview: asString(r[2]),
			updated:  parseTime(asString(r[3])),
		}
		books = append(books, bk)
		byID[bk.id] = bk
	}
	if len(books) == 0 {
		return books, nil
	}

	files, err := query(
		`SELECT f.book_id, f.id, f.format FROM streamline_book_files f
		WHERE f.slot = 'ebook' AND f.book_id IN
			(SELECT b.id FROM streamline_books b WHERE `+scope+`)
		ORDER BY f.id`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	for _, r := range files {
		if bk := byID[asInt(r[0])]; bk != nil {
			bk.files = append(
				bk.files,
				bookFile{id: asInt(r[1]), format: asString(r[2])},
			)
		}
	}

	makers, err := query(
		`SELECT c.book_id, a.name FROM streamline_book_contributions c
		JOIN streamline_authors a ON a.id = c.author_id
		WHERE c.role IN `+makerRoles+` AND c.book_id IN
			(SELECT b.id FROM streamline_books b WHERE `+scope+`)
		ORDER BY c.id`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	for _, r := range makers {
		bk := byID[asInt(r[0])]
		if name := asString(r[1]); bk != nil && !slices.Contains(bk.authors, name) {
			bk.authors = append(bk.authors, name)
		}
	}
	return books, nil
}

func acquisition(id, title, self string, books []*book) (sdk.Response, error) {
	f := &feed{
		ID:    id,
		Title: title,
		Links: []link{
			selfLink(self, kindAcquisition),
			{Rel: "start", Href: "/opds", Type: navigationType},
		},
	}
	for _, bk := range books {
		e := bookEntry(bk)
		if e == nil {
			continue
		}
		f.Entries = append(f.Entries, *e)
		f.Updated = newest(f.Updated, bk.updated)
	}
	return render(kindAcquisition, f)
}

func bookEntry(bk *book) *entry {
	best := bestFile(bk.files)
	if best == nil {
		return nil
	}
	e := &entry{
		ID:      fmt.Sprintf("urn:streamline:book:%d", bk.id),
		Title:   bk.title,
		Updated: atomTime(bk.updated),
		Links: []link{
			{
				Rel: relAcquisition,
				Href: fmt.Sprintf(
					"/opds/download/%d/%s", bk.id, strings.ToLower(best.format),
				),
				Type: ebookContentType(best.format),
			},
			{
				Rel:  relImage,
				Href: fmt.Sprintf("/opds/cover/%d", bk.id),
				Type: "image/jpeg",
			},
		},
	}
	for _, name := range bk.authors {
		e.Authors = append(e.Authors, atomAuthor{Name: name})
	}
	if bk.overview != "" {
		e.Content = &content{Type: "text", Text: bk.overview}
	}
	return e
}

func bestFile(files []bookFile) *bookFile {
	rank := func(format string) int {
		if i := slices.Index(ebookLadder, strings.ToUpper(format)); i >= 0 {
			return i
		}
		return len(ebookLadder)
	}
	var best *bookFile
	for i := range files {
		if best == nil || rank(files[i].format) < rank(best.format) {
			best = &files[i]
		}
	}
	return best
}

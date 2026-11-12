package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type atomFeed struct {
	Title   string `xml:"title"`
	Entries []struct {
		ID    string `xml:"id"`
		Title string `xml:"title"`
		Links []struct {
			Rel  string `xml:"rel,attr"`
			Href string `xml:"href,attr"`
			Type string `xml:"type,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

func parseFeed(r reply) atomFeed {
	GinkgoHelper()
	var f atomFeed
	Expect(xml.Unmarshal(r.body, &f)).To(Succeed(), r.String())
	return f
}

func (f atomFeed) titles() []string {
	out := make([]string, 0, len(f.Entries))
	for _, e := range f.Entries {
		out = append(out, e.Title)
	}
	return out
}

var epubBody = []byte("PK-not-really-an-epub-but-bytes-all-the-same")

var _ = Describe("OPDS extension", func() {
	var (
		in   *instance
		dune *seededBook
	)

	BeforeEach(func() { in = shared })

	Describe("loading", func() {
		It("reaches running with the grants it asks for", func() {
			items, ok := in.extensions()
			Expect(ok).To(BeTrue())
			Expect(items).To(HaveLen(1))
			Expect(items[0].Name).To(Equal(extName))
			Expect(items[0].State).To(Equal("running"))
			Expect(items[0].Version).NotTo(BeEmpty())
		})

		It("holds exactly what the manifest asks for, after approval", func() {
			items, _ := in.extensions()
			Expect(items).To(HaveLen(1))
			req, got := items[0].Requested, items[0].Granted
			Expect(req.viewNames()).NotTo(BeEmpty())
			Expect(got.viewNames()).To(ConsistOf(req.viewNames()))
			Expect(got.mountPrefixes()).To(ConsistOf(req.mountPrefixes()))
			Expect(got.Services).To(ConsistOf(req.Services))
		})

		It("starts with no tokens", func() {
			Expect(in.opds("/", adminEmail, adminPassword).status).
				To(Equal(http.StatusUnauthorized))
			Expect(in.opds("/", adminEmail, "").status).
				To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("the credential card", func() {
		It("is listed and creates a token that is shown once", func() {
			u := in.newUser()

			cards := in.do(reqOpts{
				path: "/api/v1/account/extensions", bearer: u.session,
			})
			Expect(cards.status).To(Equal(http.StatusOK), cards.String())
			Expect(cards.String()).To(ContainSubstring(`"extension":"opds"`))

			before := decode[struct {
				Enabled bool `json:"enabled"`
			}](u.credential(in, http.MethodGet))
			Expect(before.Enabled).To(BeFalse())

			created := u.credential(in, http.MethodPost)
			Expect(created.status).To(Equal(http.StatusOK), created.String())
			secret := decode[struct {
				Secret   string `json:"secret"`
				Username string `json:"username"`
			}](created)
			Expect(secret.Secret).NotTo(BeEmpty())
			Expect(secret.Username).To(Equal(u.email))

			state := u.credential(in, http.MethodGet)
			Expect(state.String()).NotTo(ContainSubstring(secret.Secret))
			Expect(decode[struct {
				Enabled bool `json:"enabled"`
			}](state).Enabled).To(BeTrue())

			Expect(in.opds("/", u.email, secret.Secret).status).
				To(Equal(http.StatusOK))
		})

		It("keeps only a hash of the token", func() {
			u := in.newUser()
			secret := u.newToken(in)

			rows, err := in.extensionDB().QueryContext(context.Background(),
				"SELECT token_hash FROM tokens WHERE user_id = ?", u.id)
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()
			Expect(rows.Next()).To(BeTrue())
			var stored string
			Expect(rows.Scan(&stored)).To(Succeed())
			Expect(stored).NotTo(BeEmpty())
			Expect(stored).NotTo(ContainSubstring(secret))
		})

		It("replaces the token when rotated", func() {
			u := in.newUser()
			first := u.newToken(in)
			second := u.newToken(in)
			Expect(second).NotTo(Equal(first))
			Expect(in.opds("/", u.email, first).status).
				To(Equal(http.StatusUnauthorized))
			Expect(in.opds("/", u.email, second).status).
				To(Equal(http.StatusOK))
		})

		It("never hands two users, or two servers, the same token", func() {
			a, b := in.newUser(), in.newUser()
			first, second := a.newToken(in), b.newToken(in)
			Expect(first).NotTo(Equal(second))
			Expect(in.opds("/", a.email, second).status).
				To(Equal(http.StatusUnauthorized))

			// Each request runs a fresh instance of the module, and a guest
			// that draws from its own random stream repeats it every time.
			other := startInstance(instanceOptions{enabled: true})
			admin := &account{session: other.adminToken()}
			Expect(admin.newToken(other)).NotTo(Equal(first))
			Expect(admin.newToken(other)).NotTo(Equal(second))
		})

		It("is out of reach without a session", func() {
			r := in.do(reqOpts{
				path: "/api/v1/account/extensions/" + extName + "/credential",
			})
			Expect(r.status).To(Equal(http.StatusUnauthorized))
		})

		It("is not served on the self-authenticated mount", func() {
			u := in.newUser()
			secret := u.newToken(in)
			for _, c := range []reqOpts{
				{path: "/opds/cards/token"},
				{
					method: http.MethodPost, path: "/opds/cards/token",
					basic: [2]string{u.email, secret}, useAuth: true,
				},
			} {
				r := in.do(c)
				Expect(r.status).To(Equal(http.StatusUnauthorized), c.path)
				Expect(r.String()).NotTo(ContainSubstring("secret"))
			}
		})
	})

	Describe("browsing and downloading", func() {
		var (
			reader *account
			secret string
			other  *seededBook
		)

		BeforeEach(func() {
			dune = in.seedBook("Dune Messiah", "Frank Herbert")
			in.seedFile(dune, "ebook", "EPUB", "dune.epub", epubBody)
			other = in.seedBook("Audio Only Tale", "Voice Actor")
			in.seedFile(other, "audiobook", "M4B", "tale.m4b", []byte("m4b"))
			reader = in.newUser()
			secret = reader.newToken(in)
		})

		get := func(path string) reply {
			GinkgoHelper()
			return in.opds(path, reader.email, secret)
		}

		It("serves the root catalog with navigation and search", func() {
			r := get("/")
			Expect(r.status).To(Equal(http.StatusOK))
			Expect(r.header.Get("Content-Type")).
				To(ContainSubstring("kind=navigation"))
			Expect(parseFeed(r).titles()).
				To(ContainElements("Authors", "Recently added"))
			Expect(r.String()).To(ContainSubstring("/opds/search.xml"))

			desc := get("/search.xml")
			Expect(desc.status).To(Equal(http.StatusOK))
			Expect(desc.header.Get("Content-Type")).
				To(ContainSubstring("opensearchdescription"))
		})

		It("lists authors and an author's books, ebooks only", func() {
			authors := get("/authors")
			Expect(authors.status).To(Equal(http.StatusOK))
			feed := parseFeed(authors)
			Expect(feed.titles()).To(ContainElement("Frank Herbert"))
			Expect(feed.titles()).NotTo(ContainElement("Voice Actor"))

			var href string
			for _, e := range feed.Entries {
				if e.Title == "Frank Herbert" {
					href = e.Links[0].Href
				}
			}
			Expect(href).To(HavePrefix("/opds/authors/"))
			books := get(strings.TrimPrefix(href, "/opds"))
			Expect(books.status).To(Equal(http.StatusOK))
			Expect(parseFeed(books).titles()).To(ContainElement("Dune Messiah"))
		})

		It("lists recent ebooks with an acquisition link", func() {
			r := get("/recent")
			Expect(r.status).To(Equal(http.StatusOK))
			Expect(r.header.Get("Content-Type")).
				To(ContainSubstring("kind=acquisition"))
			feed := parseFeed(r)
			Expect(feed.titles()).To(ContainElement("Dune Messiah"))
			Expect(feed.titles()).NotTo(ContainElement("Audio Only Tale"))
			Expect(r.String()).To(ContainSubstring(
				"/opds/download/" + itoa(dune.id) + "/epub"))
		})

		It("searches titles and authors, wildcards as text", func() {
			byTitle := parseFeed(get("/search?q=messiah"))
			Expect(byTitle.titles()).NotTo(BeEmpty())
			Expect(byTitle.titles()).To(HaveEach("Dune Messiah"))

			byAuthor := parseFeed(get("/search?q=herbert"))
			Expect(byAuthor.titles()).NotTo(BeEmpty())
			Expect(byAuthor.titles()).To(HaveEach("Dune Messiah"))

			Expect(parseFeed(get("/search?q=zzzz")).titles()).To(BeEmpty())
			Expect(parseFeed(get("/search")).titles()).To(BeEmpty())

			Expect(parseFeed(get("/search?q=%25")).titles()).To(BeEmpty())
			Expect(parseFeed(get("/search?q=_")).titles()).To(BeEmpty())

			quote := get("/search?q=" + "'%20OR%201=1%20--")
			Expect(quote.status).To(Equal(http.StatusOK))
			Expect(parseFeed(quote).titles()).To(BeEmpty())
		})

		It("downloads the file with range support", func() {
			r := get("/download/" + itoa(dune.id) + "/epub")
			Expect(r.status).To(Equal(http.StatusOK))
			Expect(r.body).To(Equal(epubBody))
			Expect(r.header.Get("Content-Type")).To(Equal("application/epub+zip"))
			Expect(r.header.Get("Content-Disposition")).
				To(ContainSubstring(`filename="Dune Messiah - Frank Herbert.epub"`))

			part := in.do(reqOpts{
				path:    "/opds/download/" + itoa(dune.id) + "/epub",
				basic:   [2]string{reader.email, secret},
				useAuth: true,
				headers: map[string]string{"Range": "bytes=0-1"},
			})
			Expect(part.status).To(Equal(http.StatusPartialContent))
			Expect(part.body).To(Equal(epubBody[:2]))

			ranged := func(spec string) reply {
				GinkgoHelper()
				return in.do(reqOpts{
					path:    "/opds/download/" + itoa(dune.id) + "/epub",
					basic:   [2]string{reader.email, secret},
					useAuth: true,
					headers: map[string]string{"Range": spec},
				})
			}
			suffix := ranged("bytes=-4")
			Expect(suffix.status).To(Equal(http.StatusPartialContent))
			Expect(suffix.body).To(Equal(epubBody[len(epubBody)-4:]))
			Expect(ranged("bytes=9999-").status).
				To(Equal(http.StatusRequestedRangeNotSatisfiable))
		})

		It("serves a cover from the poster cache", func() {
			jpeg := []byte("\xff\xd8\xff\xe0cover")
			in.seedPoster(dune, jpeg)
			r := get("/cover/" + itoa(dune.id))
			Expect(r.status).To(Equal(http.StatusOK))
			Expect(r.body).To(Equal(jpeg))
		})

		It("refuses a write", func() {
			r := in.do(reqOpts{
				method: http.MethodPost, path: "/opds/recent",
				basic: [2]string{reader.email, secret}, useAuth: true,
			})
			Expect(r.status).To(Equal(http.StatusMethodNotAllowed))
		})

		It("answers 404 for what does not exist", func() {
			Expect(get("/nope").status).To(Equal(http.StatusNotFound))
			Expect(get("/authors/999999").status).To(Equal(http.StatusNotFound))
			Expect(get("/authors/x").status).To(Equal(http.StatusNotFound))
		})
	})

	Describe("authentication", func() {
		var (
			dune   *seededBook
			reader *account
			secret string
			paths  []string
		)

		BeforeEach(func() {
			dune = in.seedBook("Children of Dune", "Frank Herbert")
			in.seedFile(dune, "ebook", "EPUB", "children.epub", epubBody)
			reader = in.newUser()
			secret = reader.newToken(in)
			id := itoa(dune.id)
			paths = []string{
				"/", "/authors", "/authors/1", "/recent", "/search?q=dune",
				"/search.xml", "/cover/" + id, "/download/" + id + "/epub",
				"/anything-else",
			}
		})

		requireAll401 := func(user, pass string) {
			GinkgoHelper()
			for _, p := range paths {
				r := in.opds(p, user, pass)
				Expect(r.status).To(Equal(http.StatusUnauthorized), p)
				Expect(r.header.Get("WWW-Authenticate")).To(HavePrefix("Basic"), p)
				Expect(r.String()).NotTo(ContainSubstring("Children of Dune"), p)
			}
		}

		It("asks for credentials on every path when none are sent", func() {
			for _, p := range paths {
				r := in.do(reqOpts{path: "/opds" + p})
				Expect(r.status).To(Equal(http.StatusUnauthorized), p)
				Expect(r.header.Get("WWW-Authenticate")).To(HavePrefix("Basic"), p)
			}
		})

		It("refuses a wrong password", func() {
			requireAll401(reader.email, secret+"x")
		})

		It("refuses the account password in place of the token", func() {
			requireAll401(reader.email, reader.password)
		})

		It("refuses an unknown user", func() {
			requireAll401("nobody@opds.test", secret)
		})

		It("refuses an account that never made a token", func() {
			bare := in.newUser()
			requireAll401(bare.email, secret)
			requireAll401(bare.email, bare.password)
			requireAll401(bare.email, "")
		})

		It("refuses a bearer or malformed Authorization header", func() {
			for _, h := range []string{
				"Bearer " + secret, "Basic", "Basic !!!", "Basic Zm9v",
			} {
				r := in.do(reqOpts{
					path: "/opds/", headers: map[string]string{"Authorization": h},
				})
				Expect(r.status).To(Equal(http.StatusUnauthorized), h)
			}
		})

		It("refuses the session token as a reader password", func() {
			requireAll401(reader.email, reader.session)
		})

		It("does not let one user's token open another user's name", func() {
			second := in.newUser()
			secondSecret := second.newToken(in)
			requireAll401(reader.email, secondSecret)
			requireAll401(second.email, secret)
			Expect(in.opds("/", second.email, secondSecret).status).
				To(Equal(http.StatusOK))
		})

		It("refuses a revoked token on every path, downloads included", func() {
			Expect(in.opds("/download/"+itoa(dune.id)+"/epub", reader.email, secret).
				status).To(Equal(http.StatusOK))

			Expect(reader.credential(in, http.MethodDelete).status).
				To(Equal(http.StatusNoContent))
			requireAll401(reader.email, secret)

			state := decode[struct {
				Enabled bool `json:"enabled"`
			}](reader.credential(in, http.MethodGet))
			Expect(state.Enabled).To(BeFalse())
		})

		It("stops serving a deleted user, and not to a namesake either", func() {
			Expect(in.opds("/", reader.email, secret).status).
				To(Equal(http.StatusOK))

			del := in.do(reqOpts{
				method: http.MethodDelete, bearer: in.adminToken(),
				path: "/api/v1/users/" + itoa(int64(reader.id)),
			})
			Expect(del.status).To(Equal(http.StatusNoContent), del.String())
			requireAll401(reader.email, secret)

			again := in.do(reqOpts{
				method: http.MethodPost, path: "/api/v1/users",
				bearer: in.adminToken(),
				body: map[string]string{
					"email": reader.email, "password": userPassword,
					"role": "member",
				},
			})
			Expect(again.status).To(Equal(http.StatusCreated), again.String())
			requireAll401(reader.email, secret)
			requireAll401(reader.email, userPassword)
		})

		It("refuses a locked account until the lock expires", func() {
			in.exec(
				"UPDATE users SET locked_until = ? WHERE id = ?",
				time.Now().Add(time.Hour), reader.id,
			)
			requireAll401(reader.email, secret)

			in.exec(
				"UPDATE users SET locked_until = ? WHERE id = ?",
				time.Now().Add(-time.Hour), reader.id,
			)
			Expect(in.opds("/", reader.email, secret).status).
				To(Equal(http.StatusOK))
		})

		It("follows an account's email and drops the old name", func() {
			newEmail := "renamed-" + reader.email
			patch := in.do(reqOpts{
				method: http.MethodPatch, bearer: in.adminToken(),
				path: "/api/v1/users/" + itoa(int64(reader.id)),
				body: map[string]string{"email": newEmail},
			})
			Expect(patch.status).To(Equal(http.StatusOK), patch.String())

			requireAll401(reader.email, secret)
			Expect(in.opds("/", newEmail, secret).status).To(Equal(http.StatusOK))
		})

		It("matches the user name case-insensitively but never the token", func() {
			Expect(in.opds("/", strings.ToUpper(reader.email), secret).status).
				To(Equal(http.StatusOK))
			requireAll401(strings.ToUpper(reader.email), strings.ToUpper(secret))
		})
	})

	Describe("downloads", func() {
		var (
			reader *account
			secret string
			book   *seededBook
			files  map[string]int64
		)

		BeforeEach(func() {
			book = in.seedBook("Steerable", "Test Author")
			files = map[string]int64{
				"epub": in.seedFile(book, "ebook", "EPUB", "a.epub", epubBody),
			}
			in.seedFile(book, "audiobook", "M4B", "a.m4b", []byte("audio"))
			reader = in.newUser()
			secret = reader.newToken(in)
		})

		download := func(path string) reply {
			GinkgoHelper()
			return in.opds(path, reader.email, secret)
		}

		It("answers only for a book id and a format it holds", func() {
			id := itoa(book.id)
			Expect(download("/download/" + id + "/epub").status).
				To(Equal(http.StatusOK))
			Expect(download("/download/" + id + "/EPUB").status).
				To(Equal(http.StatusOK))
			Expect(download("/download/" + id + "/pdf").status).
				To(Equal(http.StatusNotFound))
			Expect(download("/download/" + id + "/m4b").status).
				To(Equal(http.StatusNotFound))
			Expect(download("/download/999999/epub").status).
				To(Equal(http.StatusNotFound))
		})

		It("cannot be pointed at a path", func() {
			id := itoa(book.id)
			for _, p := range []string{
				"/download/" + id + "/%2e%2e%2f%2e%2e%2fetc%2fpasswd",
				"/download/" + id + "/epub/extra",
				"/download/" + id + "/" + files2path(in, book),
				"/download/" + id,
				"/download/",
				"/download/x/epub",
				"/download/-1/epub",
				"/download/" + id + "/epub%00.txt",
			} {
				r := download(p)
				Expect(r.status).To(Equal(http.StatusNotFound), p)
				Expect(r.body).NotTo(Equal(epubBody), p)
			}
			// The client cleans these before the request leaves, so the
			// server sees an ordinary path; none may reach the file or the
			// filesystem.
			for _, p := range []string{
				"/download/" + id + "/../../../etc/passwd",
				"/download/" + id + "/epub/../../etc/passwd",
			} {
				r := download(p)
				Expect(r.status).NotTo(Equal(http.StatusOK), p)
				Expect(r.String()).NotTo(ContainSubstring("root:"), p)
			}
		})

		It("ignores a path the client supplies in the query", func() {
			r := download(
				"/download/" + itoa(book.id) + "/epub?path=/etc/passwd&file=1",
			)
			Expect(r.status).To(Equal(http.StatusOK))
			Expect(r.body).To(Equal(epubBody))
		})

		It("stops serving a file the library marks missing", func() {
			in.exec(`UPDATE media_files SET missing_since = ? WHERE id = ?`,
				timeNow(), files["epub"])
			Expect(download("/download/" + itoa(book.id) + "/epub").status).
				To(Equal(http.StatusNotFound))
		})

		It("stops serving a file whose absence the library confirmed", func() {
			in.exec(`UPDATE media_files SET missing_since = ?,
				missing_confirmed_at = ? WHERE id = ?`,
				timeNow(), timeNow(), files["epub"])
			id := itoa(book.id)
			Expect(download("/download/" + id + "/epub").status).
				To(Equal(http.StatusNotFound))
			Expect(download("/recent").String()).NotTo(ContainSubstring(
				"/opds/download/" + id + "/epub"))
		})
	})

	Describe("installing", func() {
		It("waits for approval, and runs nothing until then", func() {
			raw := startInstance(instanceOptions{
				enabled: true, settleOn: "needs_approval",
			})
			items, _ := raw.extensions()
			Expect(items).To(HaveLen(1))
			Expect(items[0].Requested.viewNames()).NotTo(BeEmpty())
			Expect(items[0].Granted.viewNames()).To(BeEmpty())

			anon := raw.opds("/", adminEmail, adminPassword)
			Expect(anon.status).To(BeElementOf(
				http.StatusNotFound, http.StatusFound))

			r := raw.do(reqOpts{
				method: http.MethodPost, bearer: raw.adminToken(),
				path: "/api/v1/extensions/" + extName + "/approve",
			})
			Expect(r.status).To(Equal(http.StatusOK), r.String())
			Expect(raw.waitState("running")).To(Succeed())
			user := raw.newUser()
			Expect(raw.opds("/", user.email, user.newToken(raw)).status).
				To(Equal(http.StatusOK))
		})

		It("narrows a grant wider than the manifest on approval", func() {
			wide := startInstance(instanceOptions{
				enabled: true, settleOn: "running",
				grants: &grantSet{
					mounts: []string{"/opds"},
					views: []string{
						"users",
						"user_logins",
						"books",
						"book_series",
						"authors",
						"book_contributions",
						"book_files",
						"artists",
					},
					services: []string{"serve_file", "serve_poster"},
				},
			})
			items, _ := wide.extensions()
			Expect(items[0].Granted.viewNames()).To(ContainElement("artists"))

			r := wide.do(reqOpts{
				method: http.MethodPost, bearer: wide.adminToken(),
				path: "/api/v1/extensions/" + extName + "/approve",
			})
			Expect(r.status).To(Equal(http.StatusOK), r.String())
			items, _ = wide.extensions()
			Expect(items[0].Granted.viewNames()).
				To(ConsistOf(items[0].Requested.viewNames()))
		})

		It("refuses a module that does not match its pinned sha256", func() {
			bad := startInstance(instanceOptions{
				enabled: true, settleOn: "failed",
				sha256: strings.Repeat("0", 64),
			})
			items, _ := bad.extensions()
			Expect(items[0].Reason).To(ContainSubstring("sha256"))
		})

		It("accepts a module that matches its pinned sha256", func() {
			sum := sha256.Sum256(mustRead(wasmPath()))
			ok := startInstance(instanceOptions{
				enabled: true, sha256: hex.EncodeToString(sum[:]),
			})
			items, _ := ok.extensions()
			Expect(items[0].State).To(Equal("running"))
		})
	})

	Describe("a stopped extension", func() {
		It("answers 404 on its mount, credentials or not", func() {
			// A stopped entry's mount is known from its grants; before any
			// approval the host has no mount to answer for.
			off := startInstance(instanceOptions{
				enabled: false, grants: &grantSet{
					mounts: []string{"/opds"},
					views: []string{
						"users",
						"user_logins",
						"books",
						"book_series",
						"authors",
						"book_contributions",
						"book_files",
					},
					services: []string{"serve_file", "serve_poster"},
				},
			})

			state := decode[struct {
				Items []struct {
					State string `json:"state"`
				} `json:"items"`
			}](off.do(reqOpts{
				path: "/api/v1/extensions", bearer: off.adminToken(),
			}))
			Expect(state.Items).To(HaveLen(1))
			Expect(state.Items[0].State).To(Equal("disabled"))

			for _, p := range []string{"/opds/", "/opds/download/1/epub"} {
				signedIn := off.do(reqOpts{
					path: p,
					headers: map[string]string{
						"Cookie": "streamline_session=" + off.adminToken(),
					},
				})
				Expect(signedIn.status).To(Equal(http.StatusNotFound), p)
			}

			Expect(off.opds("/", adminEmail, adminPassword).status).
				To(Equal(http.StatusNotFound))
		})
	})
})

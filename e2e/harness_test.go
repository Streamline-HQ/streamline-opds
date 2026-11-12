package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "modernc.org/sqlite"
)

const (
	adminEmail    = "admin@opds.test"
	adminPassword = "E2e-Admin-Passw0rd!"
	userPassword  = "E2e-User-Passw0rd!1"
	scratchRoot   = "/tmp/streamline-opds"
	extName       = "opds"
	bootAttempts  = 3
)

var grantedViews = []string{
	"users", "user_logins", "books", "book_series", "authors",
	"book_contributions", "book_files",
}

type instanceOptions struct {
	enabled bool
}

type instance struct {
	dir    string
	base   string
	cmd    *exec.Cmd
	done   chan struct{}
	client *http.Client
	db     *sql.DB
	admin  string
	nextIP atomic.Uint32
	nextID atomic.Int64
}

func freePort() int {
	GinkgoHelper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	Expect(ok).To(BeTrue())
	return addr.Port
}

func wasmPath() string {
	GinkgoHelper()
	p, err := filepath.Abs("../opds.wasm")
	Expect(err).NotTo(HaveOccurred())
	Expect(p).To(BeAnExistingFile(), "run task build first")
	return p
}

func startInstance(opts instanceOptions) *instance {
	GinkgoHelper()
	Expect(os.MkdirAll(scratchRoot, 0o750)).To(Succeed())
	dir, err := os.MkdirTemp(scratchRoot, "run-")
	Expect(err).NotTo(HaveOccurred())

	var lastErr error
	for range bootAttempts {
		in := &instance{
			dir: dir,
			client: &http.Client{
				Timeout: 15 * time.Second,
				CheckRedirect: func(*http.Request, []*http.Request) error {
					return http.ErrUseLastResponse
				},
			},
		}
		in.nextIP.Store(1)
		in.nextID.Store(100)
		lastErr = in.boot(opts)
		if lastErr == nil {
			lastErr = in.waitSettled(opts)
		}
		if lastErr == nil {
			DeferCleanup(func() { Expect(os.RemoveAll(dir)).To(Succeed()) })
			return in
		}
		in.stop()
	}
	Fail(fmt.Sprintf("streamline did not boot: %v", lastErr))
	return nil
}

func (in *instance) config(port int, opts instanceOptions) []byte {
	GinkgoHelper()
	lib := func(name string) string {
		p := filepath.Join(in.dir, "lib", name)
		Expect(os.MkdirAll(p, 0o750)).To(Succeed())
		return p
	}
	cfg := map[string]any{
		"data_dir": filepath.Join(in.dir, "data"),
		"server": map[string]any{
			"host":            "127.0.0.1",
			"port":            port,
			"trusted_proxies": []string{"127.0.0.1/32"},
		},
		"auth": map[string]any{
			"session_secret":    "e2e-session-secret-e2e-session-secret",
			"api_failure_limit": "0",
			"seed_admin": map[string]any{
				"email":    adminEmail,
				"password": adminPassword,
			},
		},
		"backup": map[string]any{"pre_migration": false},
		"library": map[string]any{
			"ebook_path":     lib("ebooks"),
			"audiobook_path": lib("audiobooks"),
			"movie_path":     lib("movies"),
			"series_path":    lib("series"),
			"music_path":     lib("music"),
			"download_path":  lib("downloads"),
		},
		"extensions": []any{map[string]any{
			"name":    extName,
			"source":  wasmPath(),
			"enabled": opts.enabled,
			"grants": map[string]any{
				"mounts":   []string{"/opds"},
				"views":    grantedViews,
				"services": []string{"serve_file", "serve_poster"},
			},
		}},
	}
	b, err := json.Marshal(cfg)
	Expect(err).NotTo(HaveOccurred())
	return b
}

func (in *instance) boot(opts instanceOptions) error {
	GinkgoHelper()
	port := freePort()
	in.base = fmt.Sprintf("http://127.0.0.1:%d", port)
	cfgPath := filepath.Join(in.dir, "config.yaml")
	Expect(os.WriteFile(cfgPath, in.config(port, opts), 0o600)).To(Succeed())

	bin := os.Getenv("STREAMLINE_BIN")
	cmd := exec.Command(bin, "--config", cfgPath)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	cmd.Env = append(os.Environ(), "TMPDIR="+in.dir)
	if err := cmd.Start(); err != nil {
		return err
	}
	in.cmd = cmd
	in.done = make(chan struct{})
	go func() {
		defer GinkgoRecover()
		_ = cmd.Wait()
		close(in.done)
	}()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-in.done:
			return fmt.Errorf("streamline exited during boot")
		default:
		}
		resp, err := in.client.Get(in.base + "/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return in.open()
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("no /health within 60s")
}

func (in *instance) open() error {
	dsn := "file:" + filepath.Join(in.dir, "data", "streamline.db") +
		"?_pragma=busy_timeout(10000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	in.db = db
	return nil
}

func (in *instance) stop() {
	if in.db != nil {
		_ = in.db.Close()
	}
	if in.cmd == nil || in.cmd.Process == nil {
		return
	}
	select {
	case <-in.done:
		return
	default:
	}
	_ = in.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-in.done:
	case <-time.After(10 * time.Second):
		_ = in.cmd.Process.Kill()
		<-in.done
	}
}

type reqOpts struct {
	method  string
	path    string
	headers map[string]string
	body    any
	bearer  string
	basic   [2]string
	useAuth bool
}

type reply struct {
	status int
	header http.Header
	body   []byte
}

func (r reply) String() string { return string(r.body) }

func (in *instance) do(o reqOpts) reply {
	GinkgoHelper()
	var rdr io.Reader
	if o.body != nil {
		b, err := json.Marshal(o.body)
		Expect(err).NotTo(HaveOccurred())
		rdr = bytes.NewReader(b)
	}
	method := o.method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(
		context.Background(), method, in.base+o.path, rdr,
	)
	Expect(err).NotTo(HaveOccurred())
	if o.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if o.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+o.bearer)
	}
	if o.useAuth {
		req.SetBasicAuth(o.basic[0], o.basic[1])
	}
	for k, v := range o.headers {
		req.Header.Set(k, v)
	}
	resp, err := in.client.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	return reply{status: resp.StatusCode, header: resp.Header, body: b}
}

func (in *instance) login(email, password string) string {
	GinkgoHelper()
	// Login is limited to five attempts per client address; the instance
	// trusts loopback as a proxy, so each login names its own client.
	ip := fmt.Sprintf("10.9.0.%d", in.nextIP.Add(1))
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, in.base+"/auth/login",
		strings.NewReader(mustJSON(map[string]string{
			"email": email, "password": password,
		})),
	)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", ip)
	resp, err := in.client.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
	for _, c := range resp.Cookies() {
		if c.Name == "streamline_session" {
			return c.Value
		}
	}
	Fail("login set no session cookie")
	return ""
}

func mustJSON(v any) string {
	GinkgoHelper()
	b, err := json.Marshal(v)
	Expect(err).NotTo(HaveOccurred())
	return string(b)
}

func decode[T any](r reply) T {
	GinkgoHelper()
	var v T
	Expect(json.Unmarshal(r.body, &v)).To(Succeed(), r.String())
	return v
}

func (in *instance) adminToken() string {
	GinkgoHelper()
	if in.admin == "" {
		in.admin = in.login(adminEmail, adminPassword)
	}
	return in.admin
}

type account struct {
	id       int
	email    string
	password string
	session  string
}

func (in *instance) newUser() *account {
	GinkgoHelper()
	email := fmt.Sprintf("reader%d@opds.test", in.nextID.Add(1))
	r := in.do(reqOpts{
		method: http.MethodPost, path: "/api/v1/users", bearer: in.adminToken(),
		body: map[string]string{
			"email": email, "password": userPassword, "role": "member",
		},
	})
	Expect(r.status).To(Equal(http.StatusCreated), r.String())
	created := decode[struct {
		ID int `json:"id"`
	}](r)
	return &account{
		id: created.ID, email: email, password: userPassword,
		session: in.login(email, userPassword),
	}
}

func (a *account) credential(in *instance, method string) reply {
	GinkgoHelper()
	return in.do(reqOpts{
		method: method, bearer: a.session,
		path: "/api/v1/account/extensions/" + extName + "/credential",
	})
}

func (a *account) newToken(in *instance) string {
	GinkgoHelper()
	r := a.credential(in, http.MethodPost)
	Expect(r.status).To(Equal(http.StatusOK), r.String())
	secret := decode[struct {
		Secret string `json:"secret"`
	}](r).Secret
	Expect(secret).NotTo(BeEmpty())
	return secret
}

func (in *instance) opds(path, user, pass string) reply {
	GinkgoHelper()
	return in.do(reqOpts{
		path: "/opds" + path, basic: [2]string{user, pass}, useAuth: true,
	})
}

type seededBook struct {
	id     int64
	title  string
	author string
}

var seedCounter atomic.Int64

func (in *instance) exec(q string, args ...any) int64 {
	GinkgoHelper()
	res, err := in.db.ExecContext(context.Background(), q, args...)
	Expect(err).NotTo(HaveOccurred())
	id, err := res.LastInsertId()
	Expect(err).NotTo(HaveOccurred())
	return id
}

func (in *instance) seedBook(title, author string) *seededBook {
	GinkgoHelper()
	n := seedCounter.Add(1)
	now := time.Now().UTC()
	id := in.exec(
		`INSERT INTO books(create_time, update_time, hardcover_id, title,
			sort_title, author_name) VALUES(?, ?, ?, ?, ?, ?)`,
		now, now, 1000+n, title, title, author,
	)
	authorID := in.exec(
		`INSERT INTO authors(create_time, update_time, hardcover_id, name,
			sort_name) VALUES(?, ?, ?, ?, ?)`,
		now, now, 5000+n, author, author,
	)
	in.exec(
		`INSERT INTO book_contributions(create_time, update_time, role,
			author_contributions, book_contributions) VALUES(?, ?, 'author', ?, ?)`,
		now, now, authorID, id,
	)
	return &seededBook{id: id, title: title, author: author}
}

func (in *instance) seedFile(
	b *seededBook, slot, format, name string, content []byte,
) int64 {
	GinkgoHelper()
	dir := filepath.Join(in.dir, "lib", "ebooks", fmt.Sprintf("book-%d", b.id))
	Expect(os.MkdirAll(dir, 0o750)).To(Succeed())
	p := filepath.Join(dir, name)
	Expect(os.WriteFile(p, content, 0o600)).To(Succeed())
	now := time.Now().UTC()
	return in.exec(
		`INSERT INTO media_files(create_time, update_time, path, size, quality,
			format, book_kind, book_media_files) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		now, now, p, len(content), format, strings.ToLower(format), slot, b.id,
	)
}

func (in *instance) seedPoster(b *seededBook, content []byte) {
	GinkgoHelper()
	dir := filepath.Join(in.dir, "data", "posters", "books",
		fmt.Sprint(b.id))
	Expect(os.MkdirAll(dir, 0o750)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(dir, "poster.jpg"), content, 0o600)).
		To(Succeed())
}

func (in *instance) extensionDB() *sql.DB {
	GinkgoHelper()
	p := filepath.Join(in.dir, "data", "extensions", extName, "data.db")
	Expect(p).To(BeAnExistingFile())
	db, err := sql.Open("sqlite", "file:"+p+"?mode=ro&_pragma=busy_timeout(5000)")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(db.Close)
	return db
}

func itoa(n int64) string { return fmt.Sprint(n) }

func timeNow() time.Time { return time.Now().UTC() }

func files2path(in *instance, b *seededBook) string {
	GinkgoHelper()
	var p string
	err := in.db.QueryRowContext(context.Background(),
		`SELECT path FROM media_files WHERE book_media_files = ? AND
			book_kind = 'ebook'`, b.id).Scan(&p)
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimPrefix(p, "/")
}

// The manager reports `starting` while it loads the modules after boot, and
// /health answers before that is over.
func (in *instance) waitSettled(opts instanceOptions) error {
	GinkgoHelper()
	want := "running"
	if !opts.enabled {
		want = "disabled"
	}
	var last string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		r := in.do(reqOpts{path: "/api/v1/extensions", bearer: in.adminToken()})
		var list struct {
			Items []struct {
				State  string `json:"state"`
				Reason string `json:"reason"`
			} `json:"items"`
		}
		if r.status == http.StatusOK && json.Unmarshal(r.body, &list) == nil &&
			len(list.Items) == 1 {
			last = list.Items[0].State + " " + list.Items[0].Reason
			if list.Items[0].State == want {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("extension never became %s: %s", want, last)
}

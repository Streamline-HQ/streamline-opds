<div align="center">

# streamline-opds-ext

**Your [Streamline](https://github.com/DataHearth/streamline) ebook library, in every e-reader app.**

An OPDS 1.2 catalog, shipped as a sandboxed WebAssembly extension.

[![CI](https://github.com/Streamline-HQ/streamline-opds-ext/actions/workflows/ci.yaml/badge.svg)](https://github.com/Streamline-HQ/streamline-opds-ext/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/Streamline-HQ/streamline-opds-ext?sort=semver)](https://github.com/Streamline-HQ/streamline-opds-ext/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/Streamline-HQ/streamline-opds-ext)](go.mod)
[![OPDS 1.2](https://img.shields.io/badge/OPDS-1.2-blue)](https://specs.opds.io/opds-1.2)
[![License: GPL-3.0](https://img.shields.io/github/license/Streamline-HQ/streamline-opds-ext)](LICENSE)

[Quick start](#quick-start) · [Catalog](#the-catalog) · [Security](#security-model) · [Operations](#operations) · [Troubleshooting](#troubleshooting) · [Development](#development) · [User guide](docs/wiki/OPDS.md)

</div>

---

Streamline loads `opds.wasm` and serves it at `/extensions/opds`. Readers such as KOReader, Moon+ Reader, Librera, Marvin and Foliate add that URL as a catalog. They can then browse by author, see what was added recently, search, and download books with their covers. All of this goes through the permissions an admin approves. The module runs sandboxed in Streamline's WebAssembly runtime: it gets no folder, and it reads data only through the views it was granted.

## Contents

- [Features](#features)
- [Compatibility](#compatibility)
- [Quick start](#quick-start)
- [Permissions](#permissions)
- [The catalog](#the-catalog)
- [Security model](#security-model)
- [Operations](#operations)
- [Upgrading](#upgrading)
- [Troubleshooting](#troubleshooting)
- [Limitations](#limitations)
- [Development](#development)
- [Releasing](#releasing)
- [Contributing](#contributing)
- [Streamline repositories](#streamline-repositories)
- [License](#license)

## Features

- **OPDS 1.2 navigation and acquisition feeds** that any compliant reader understands.
- **Browse by author**: everyone credited as author or writer on a book that has an ebook, sorted by sort name.
- **Recently added**: the 50 newest ebooks.
- **Search** over titles and author names, discoverable through OpenSearch. `%` and `_` are matched as plain characters.
- **The best format first**: each book links its best file (EPUB, then AZW3, MOBI, PDF, CBZ, CBR). Any other format the book holds is one URL away.
- **Covers** and **descriptions**: covers come from Streamline's poster cache. Entries show each book's overview and credit its authors, writers, artists, colorists and cover artists.
- **Resumable downloads**: Streamline serves the file itself, with HTTP range support, under a clean `Title - Author.ext` name.
- **Per-account reader tokens**: each account manages its own token on its account page, and sees when it was last used and by which app.
- **Ebooks only**: audiobooks are never listed.

## Compatibility

| streamline-opds-ext | Streamline extension contract | Served at |
| --- | --- | --- |
| **v2.x** (current) | 2 | `/extensions/opds` |
| v1.x | 1 | `/opds` |

Pick the major version that matches the extension contract your Streamline release implements. The extension works with any OPDS 1.2 client that supports HTTP Basic authentication.

## Quick start

**1. Declare the extension** in Streamline's config. Take the URL of `opds.wasm` and its SHA-256 from the [latest release](https://github.com/Streamline-HQ/streamline-opds-ext/releases/latest). The release notes print a ready-to-paste entry, and the checksum is also in `opds.wasm.sha256`.

```yaml
extensions:
  - name: opds
    source: https://github.com/Streamline-HQ/streamline-opds-ext/releases/download/vX.Y.Z/opds.wasm
    sha256: <the release's SHA-256>
    enabled: true
```

Streamline refuses a module whose bytes do not match `sha256`.

**2. Approve it** in **Settings → Extensions**, or with `POST /api/v1/extensions/opds/approve`. Until an admin approves it, the extension stays in `needs approval` and runs nothing. The approval screen lists exactly what it asks for (see [Permissions](#permissions)).

**3. Create a token.** Each user opens **Account → Players & readers** and creates one on the **OPDS** card. The token is shown **once**.

**4. Add the catalog** in the reader app:

| Field | Value |
| --- | --- |
| URL | `https://<your-streamline>/extensions/opds` |
| User name | the Streamline account's email address |
| Password | the token from step 3 |

## Permissions

The manifest asks for the following, and nothing else. On approval, Streamline narrows any wider grant to this list.

| Asks for | Why |
| --- | --- |
| The `/` mount (all of `/extensions/opds`), checking credentials itself | Reader apps send HTTP Basic, not a Streamline session |
| Users' names, roles and email addresses (`users`, `user_logins`) | To find the account a reader signs in as, and to refuse a locked one |
| The book library: books, series, authors, contributions (`books`, `book_series`, `authors`, `book_contributions`) | To build the catalog (`book_series` is granted, but the current feeds do not read it) |
| Which book files exist (`book_files`) | To list only books with an ebook and pick the file to send |
| `serve_file` | To send a book file |
| `serve_poster` | To send a cover |
| One account card (`credential`) | The OPDS card under Account → Players & readers |

It asks for **no folder, no `ffmpeg` and no library scan**.

## The catalog

All paths are relative to `/extensions/opds` and need HTTP Basic credentials. Feed links are built from the base path Streamline serves the extension at, never from a hard-coded URL.

| Path | Returns |
| --- | --- |
| `/` | Root navigation feed: Authors, Recently added, search |
| `/authors` | Navigation feed of authors |
| `/authors/<id>` | Acquisition feed of that author's books |
| `/recent` | Acquisition feed of the 50 newest ebooks |
| `/search.xml` | OpenSearch description |
| `/search?q=<terms>` | Acquisition feed of books whose title or author name matches |
| `/cover/<book id>` | The book's cover |
| `/download/<book id>/<format>` | The book's file in that format (`epub`, `azw3`, `mobi`, `pdf`, `cbz`, `cbr`, …) |

The extension answers only `GET`, and anything else gets `405`. A path that does not exist, a book with no file in the requested format, or a file the library marks missing answers `404`.

## Security model

- **Separate credentials.** Readers authenticate with the account's email address and a dedicated token. The Streamline password and session tokens are refused here, and the reader token works nowhere else in Streamline.
- **Strong tokens, hashed at rest.** A token holds 256 bits drawn from the host's entropy, mixed with the guest's own entropy and the clock. Only its SHA-256 hash is stored. The extension compares hashes in constant time.
- **Revocation takes effect at once.** Creating a new token replaces the old one. Disabling the token, locking the account or deleting it ends access on every path, covers and downloads included.
- **Bound to the account.** One account's token cannot open another account's name. The user name is matched case-insensitively; the token is matched exactly. When an account's email address changes, the old address stops working.
- **No path from the client.** A file is chosen from the library by book id and format, never from a path or a query the client sends.
- **Fails closed.** A disabled, unapproved or failed extension answers `404` to everyone, signed in or not.
- **Sandboxed.** The module runs in Streamline's WebAssembly runtime. It gets no folder, and its data access is limited to the approved list above.

To report a vulnerability, open a [private security advisory](https://github.com/Streamline-HQ/streamline-opds-ext/security/advisories/new) and see [SECURITY.md](SECURITY.md) for what is in scope. Do not open a public issue for it.

## Operations

**State.** The extension's only state is its token table, kept in its own SQLite database at `<data_dir>/extensions/opds/data.db`. The table holds one row per account: the token hash, its creation time, when it was last used and by which client. The last use is stamped at most every 5 minutes. The client name comes from the reader's `User-Agent`.

**Backups.** Include `data.db` to keep reader tokens across a restore. If you lose it, nothing in the library is lost; each user creates a new token.

**Disabling.** Set `enabled: false` or disable the extension in Settings → Extensions. `/extensions/opds` then answers `404` at once. Its database stays in place, so the tokens are still there when you enable it again.

**Logs.** Failures go to Streamline's log, for example `OPDS request failed` (with the path), `OPDS authentication failed` or `OPDS token card failed`. A client only ever sees a generic `internal error`.

**Starting from scratch.** The extension starts with **no tokens**. Nothing from the OPDS server that used to be built into Streamline carries over, so every reader app needs a new token.

## Upgrading

1. Read the [release notes](https://github.com/Streamline-HQ/streamline-opds-ext/releases). Each one names its SHA-256.
2. Replace `source` and `sha256` in the config with the new release's values.
3. Approve again if Streamline asks for it.

**From v1 to v2**: v2 needs a Streamline release that serves extensions at `/extensions/<name>` (extension contract 2). Its catalog moves from `/opds` to `/extensions/opds`, so update the catalog URL in every reader app.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| The reader keeps asking for credentials (`401`) | The user name is the account's **email address**. The password is the **reader token**, not the account password. Only the latest token works. A locked account is refused until its lock expires. |
| `/extensions/opds` answers `404` | The extension's state in Settings → Extensions: it must be approved and running. Also check the URL: v2 lives under `/extensions/opds`, not `/opds`. |
| Streamline refuses to load the module | The `sha256` in the config must match the release's `opds.wasm`. The release must also match your Streamline's extension contract (see [Compatibility](#compatibility)). |
| A book does not appear | Only books with an ebook file are listed, and audiobooks never are. An author appears only when credited as author or writer. |
| A download answers `404` | The book has no file in that format, or the library marks the file missing. Use the acquisition link from the feed, which always points to a format the book holds. |

## Limitations

- OPDS 1.2 only. OPDS 2.0 (JSON) is not served.
- Feeds are not paginated. An author's books and the search results arrive as one feed.
- No browsing by series, tag or language yet.
- Read-only: no reading progress sync, shelves or uploads.

## Development

**Requirements:** Go (the version in [`go.mod`](go.mod)) and [Task](https://taskfile.dev). The end-to-end suite also needs a Streamline binary.

```sh
task build   # build opds.wasm (GOOS=wasip1 GOARCH=wasm, -buildmode=c-shared)
task lint    # golangci-lint: the guest for wasip1, the end-to-end suite for the host
task fmt     # gofumpt + golines
task test    # build, then run the end-to-end suite
```

`task test` boots a real Streamline binary in a scratch directory, on a free port, with `opds.wasm` configured as an extension. It then drives the install flow and `/extensions/opds` over HTTP. Name the binary with `STREAMLINE_BIN`:

```sh
STREAMLINE_BIN=../streamline/streamline task test
```

The suite covers:

- loading and approval, with grant narrowing and checksum pinning;
- the token card;
- every feed, search, downloads with range requests, and covers;
- authentication: wrong, revoked, foreign and session credentials, locked, deleted and renamed accounts;
- path-injection attempts on downloads;
- the behavior of a stopped extension.

**SDK.** The extension SDK is `github.com/datahearth/streamline/extensions/sdk`, which lives in the Streamline repository. It has no tagged release yet, so `go.mod` pins it to a commit (a pseudo-version). Move it with:

```sh
go get github.com/datahearth/streamline/extensions/sdk@<commit>
```

To build against a local Streamline checkout, use a `go.work` file and do not commit it:

```sh
go work init . ./e2e && go work use ../streamline/extensions/sdk
```

### Layout

```text
.
├── main.go         manifest (mounts, views, services, account card) and routing
├── auth.go         HTTP Basic against the per-account token
├── tokens.go       token store, entropy, last-use stamping
├── cards.go        the OPDS card: status (GET), create or replace (POST), disable (DELETE)
├── catalog.go      feeds: root, authors, recent, search, format ranking
├── feed.go         Atom / OPDS rendering
├── download.go     picking and naming the file to send
├── e2e/            end-to-end suite (its own Go module, Ginkgo + Gomega)
└── docs/wiki/      user guide
```

The guest imports nothing but the SDK. The end-to-end suite is its own Go module, so its dependencies and Go tools (Ginkgo, golangci-lint) never reach the guest.

## Releasing

1. Bump `Version` in [`main.go`](main.go).
2. Merge to `main`.
3. Push a tag `vX.Y.Z` that equals that version.

The [release workflow](.github/workflows/release.yaml) then:

1. checks that the tag matches `Version`;
2. builds `opds.wasm` with `-trimpath`;
3. writes `opds.wasm.sha256`;
4. publishes a GitHub release with both files attached and a ready-to-paste config entry in its notes;
5. downloads the published file and checks it against that checksum.

Versions follow [Semantic Versioning](https://semver.org). A new major version means a new Streamline extension contract or a breaking change for readers (see [Compatibility](#compatibility)).

## Contributing

Bug reports and PRs welcome — see [CONTRIBUTING.md](CONTRIBUTING.md) for setup,
the build/test loop, conventions and the policy on AI-assisted PRs. Participation
is governed by the [Code of Conduct](CODE_OF_CONDUCT.md).

Found a vulnerability? Don't open an issue — see [SECURITY.md](SECURITY.md).

## Streamline repositories

| Repository | What it is |
| --- | --- |
| [streamline](https://github.com/DataHearth/streamline) | The server: media manager, web UI and extension host |
| [streamline-player](https://github.com/Streamline-HQ/streamline-player) | The player apps (Android TV, desktop, web) and the HLS streaming engine |
| [streamline-player-ext](https://github.com/Streamline-HQ/streamline-player-ext) | The `player` extension: the API the player apps talk to |
| [streamline-opds-ext](https://github.com/Streamline-HQ/streamline-opds-ext) | OPDS 1.2 catalog for e-reader apps |
| [streamline-subsonic-ext](https://github.com/Streamline-HQ/streamline-subsonic-ext) | Subsonic / OpenSubsonic server for music apps |

## License

[GPL-3.0](LICENSE).

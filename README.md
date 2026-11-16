# streamline-opds-ext

[![CI](https://github.com/Streamline-HQ/streamline-opds-ext/actions/workflows/ci.yaml/badge.svg)](https://github.com/Streamline-HQ/streamline-opds-ext/actions/workflows/ci.yaml)
[![Release](https://img.shields.io/github/v/release/Streamline-HQ/streamline-opds-ext)](https://github.com/Streamline-HQ/streamline-opds-ext/releases/latest)
[![License: GPL-3.0](https://img.shields.io/github/license/Streamline-HQ/streamline-opds-ext)](LICENSE)

An OPDS 1.2 catalog of your [Streamline](https://github.com/DataHearth/streamline) ebook library, for e-reader apps (KOReader, Moon+ Reader, Librera, Marvin, Foliate and others). It is a Streamline **extension**: a WebAssembly module the server loads and serves at `/extensions/opds`, not part of the Streamline binary. Ebooks only; audiobooks are not listed.

The user guide is [docs/wiki/OPDS.md](docs/wiki/OPDS.md).

## Features

- **Browse** by author, or the 50 most recently added ebooks.
- **Search** titles and author names, with an OpenSearch description at `/extensions/opds/search.xml`.
- **Download** the best format a book holds (EPUB, then AZW3, MOBI, PDF, CBZ, CBR), or any other one at `/extensions/opds/download/<book id>/<format>`.
- **Covers** served from the library.
- **Per-account tokens**: readers sign in with HTTP Basic, the account's email address and a token created under Account → Players & readers. The Streamline password never works here, and the token works nowhere else. Disabling the token, or locking or deleting the account, ends access at once.

## Compatibility

| Extension | Streamline extension contract | Served at |
| --- | --- | --- |
| v2.x | 2 | `/extensions/opds` |
| v1.x | 1 | `/opds` |

## Install

Add an entry under `extensions` in Streamline's config, with the URL of `opds.wasm` on a [release](https://github.com/Streamline-HQ/streamline-opds-ext/releases) of this repository and the SHA-256 the release notes name (it is also in `opds.wasm.sha256`):

```yaml
extensions:
  - name: opds
    source: https://github.com/Streamline-HQ/streamline-opds-ext/releases/download/vX.Y.Z/opds.wasm
    sha256: <the release's SHA-256>
    enabled: true
```

Streamline refuses a module whose bytes do not match the checksum. Then approve it in **Settings → Extensions**. The approval screen lists what it asks for: the `/` mount (all of `/extensions/opds`), which checks credentials itself; read access to users' names and email addresses and to the book library (books, series, authors, who made each book, which files exist); and the permissions to send a book file and a book picture to a reader. It asks for no folder, no `ffmpeg` and no library scan.

It starts with **no tokens**: nothing from the OPDS server that used to be built in is carried over. Each account creates its own under **Account → Players & readers**, then adds a catalogue in its reader app with:

- URL: `https://your-streamline.example/extensions/opds`
- User name: the Streamline email address
- Password: the token

## Development

Requirements: Go (the version in `go.mod`) and [Task](https://taskfile.dev).

The extension SDK is the `github.com/datahearth/streamline/extensions/sdk` module, in the Streamline repository. It has no tagged release yet, so `go.mod` pins it to a commit (a pseudo-version); move it with `go get github.com/datahearth/streamline/extensions/sdk@<commit>`. To build against a local Streamline checkout instead, use a `go.work` that is not committed: `go work init . ./e2e && go work use ../streamline/extensions/sdk`.

```sh
task build   # opds.wasm
task lint    # golangci-lint: the guest for wasip1, the end-to-end suite for the host
task fmt     # format all code
task test    # the end-to-end suite
```

`task test` boots a real Streamline binary with `opds.wasm` configured as an extension, in a scratch directory and on a free port, and drives `/extensions/opds` over HTTP. Name the binary with `STREAMLINE_BIN`:

```sh
STREAMLINE_BIN=../streamline/streamline task test
```

The end-to-end suite is its own Go module (`e2e/`), so the guest imports nothing but the SDK. Its Go tools (Ginkgo, golangci-lint) are pinned there too.

### Layout

| File | What it does |
| --- | --- |
| `main.go` | Manifest (mounts, views, services, account card) and routing |
| `auth.go` | HTTP Basic against the per-account token |
| `tokens.go` | The token store, in the extension's own database |
| `cards.go` | The OPDS card under Account → Players & readers: status, create, disable |
| `catalog.go`, `feed.go` | The OPDS feeds (root, authors, recent, search) and their Atom rendering |
| `download.go` | Picking and sending a book file |
| `e2e/` | End-to-end suite against a real Streamline binary |

## Releasing

Push a tag `vX.Y.Z` whose version equals the `Version` in `main.go`. The release workflow builds `opds.wasm` with `-trimpath`, writes `opds.wasm.sha256`, and publishes a release with both attached and the SHA-256 in its notes; it then downloads the published file and checks it against that checksum.

## License

GPL-3.0, see [LICENSE](LICENSE).

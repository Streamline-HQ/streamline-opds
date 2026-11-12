# streamline-opds

OPDS 1.2 catalog for [Streamline](https://github.com/DataHearth/streamline), as an extension: a WebAssembly module the server loads and mounts at `/opds`. Ebooks only. The user guide is [docs/wiki/OPDS.md](docs/wiki/OPDS.md).

## Install

Add an entry under `extensions` in Streamline's config, with the URL of `opds.wasm` on a release of this repository and the SHA-256 the release notes name (it is also in `opds.wasm.sha256`):

```yaml
extensions:
  - name: opds
    source: https://github.com/Streamline-HQ/streamline-opds/releases/download/vX.Y.Z/opds.wasm
    sha256: <the release's SHA-256>
    enabled: true
```

Then approve it in Settings → Extensions. The approval screen lists what it asks for: the `/opds` mount, which checks credentials itself; read access to users' names and email addresses and to the book library (books, series, authors, who made each book, which files exist); and the permissions to send a book file and a book picture to a reader. It asks for no folder, no `ffmpeg` and no library scan.

It starts with **no tokens**: nothing from the OPDS server that used to be built in is carried over. Each account creates its own under Account → Players & readers.

## Development

The extension SDK lives in the Streamline repository (`extensions/sdk`) and has no published version yet, so `go.mod` replaces it with `../streamline/extensions/sdk`. Keep a Streamline checkout beside this one, named `streamline`. Until the SDK is tagged and the `replace` dropped, the workflows in `.github/workflows` cannot resolve it on GitHub and fail there; they are written for the day it can.

```sh
task build   # opds.wasm
task lint    # golangci-lint: the guest for wasip1, the end-to-end suite for the host
task test    # the end-to-end suite
```

`task test` boots a real Streamline binary with `opds.wasm` configured as an extension, in a scratch directory and on a free port, and drives `/opds` over HTTP. Name the binary with `STREAMLINE_BIN`:

```sh
STREAMLINE_BIN=../streamline/streamline task test
```

The end-to-end suite is its own Go module (`e2e/`), so the guest imports nothing but the SDK. Its Go tools (Ginkgo, golangci-lint) are pinned there too.

## Releasing

Push a tag `vX.Y.Z` whose version equals the `Version` in `main.go`. The release workflow builds `opds.wasm` with `-trimpath`, writes `opds.wasm.sha256`, and publishes a release with both attached and the SHA-256 in its notes; it then downloads the published file and checks it against that checksum.

## License

GPL-3.0, see [LICENSE](LICENSE).

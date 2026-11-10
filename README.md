# streamline-opds

OPDS 1.2 catalog for [Streamline](https://github.com/DataHearth/streamline), as an extension: a WebAssembly module the server loads, mounted at `/opds`. Ebooks only.

## Install

In Streamline, Settings → Extensions → Add: give it the URL of `opds.wasm` from a release of this repository and its SHA-256, then review what it asks for. See the [Extensions](https://github.com/DataHearth/streamline/wiki/Extensions) wiki page.

Each account then creates its own reader password on its account page.

## Build

```sh
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o opds.wasm .
```

The extension SDK lives in the Streamline repository (`extensions/sdk`) and has no published version yet, so `go.mod` replaces it with a checkout of Streamline beside this one (`../streamline`). The replace goes once the SDK is tagged.

## License

GPL-3.0, see [LICENSE](LICENSE).

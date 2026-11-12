# OPDS

An OPDS 1.2 catalog of your ebooks, for e-reader apps (KOReader, Moon+ Reader, Librera, Marvin, Foliate and others). It is an extension: a module Streamline loads, not part of the binary. Ebooks only; audiobooks are not listed. For what an extension is, see [Extensions](Extensions).

## Adding it

Add an entry to the config, using the URL of `opds.wasm` on a release of [streamline-opds](https://github.com/Streamline-HQ/streamline-opds/releases) and the SHA-256 in that release's notes:

```yaml
extensions:
  - name: opds
    source: https://github.com/Streamline-HQ/streamline-opds/releases/download/vX.Y.Z/opds.wasm
    sha256: <the release's SHA-256>
    enabled: true
```

Streamline refuses a module whose bytes do not match the checksum. A new entry is `needs approval` until an admin approves it in **Settings → Extensions** (or with `POST /api/v1/extensions/opds/approve`). The screen lists:

| Asks for | Why |
| --- | --- |
| The `/opds` mount, checking credentials itself | Reader apps send HTTP Basic, not a Streamline session |
| See user names and roles; See users' email addresses | To find the account a reader signs in as |
| Read the book library, the book series, book authors, who made each book | To build the catalogue |
| Read which book files exist | To list only books with an ebook and find the file to send |
| Send a book file to a client | Downloads |
| Send a book or author picture to a client | Covers |

It asks for no folder, no `ffmpeg` and no library scan. It stores its tokens in its own database, `<data_dir>/extensions/opds/data.db`.

## Getting a token

It starts with **no tokens**. Nothing from the OPDS server that used to be built into Streamline was carried over, so every reader app needs a new one.

1. Sign in to Streamline and open **Account → Players & readers**.
2. On the **OPDS** card, create a token. It is shown **once**; copy it then. Creating another replaces it, and the old one stops working at once.
3. In the reader app, add a catalogue with:
   - URL: `https://your-streamline.example/opds`
   - User name: your Streamline email address
   - Password: the token

Disabling the token on the card, or deleting the account, ends access on every path, covers and downloads included. Your Streamline password never works here, and the token never works anywhere else.

## What the catalogue holds

- **Authors**: everyone credited as an author or writer on a book with an ebook, and each one's books.
- **Recently added**: the 50 newest ebooks.
- **Search**: the OpenSearch description is at `/opds/search.xml`; a query matches titles and author names.

A book with several formats links the best one (EPUB, then AZW3, MOBI, PDF, CBZ, CBR); a download for another format a book holds is `/opds/download/<book id>/<format>`. Anything else answers `404`, and a book's file is chosen from the library by its id, never from a path the client sends.

## When it is off

A disabled or failed extension's `/opds` answers `404` to a signed-in browser, and sends everyone else to the sign-in page, as any Streamline path.

## Development

See the [repository's README](https://github.com/Streamline-HQ/streamline-opds#development).

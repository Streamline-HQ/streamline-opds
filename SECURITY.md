# Security Policy

## Supported versions

| Version              | Supported          |
| -------------------- | ------------------ |
| Latest release       | :white_check_mark: |
| Anything older       | :x:                |

streamline-opds is maintained by one person. Fixes land in a new patch release
on top of the latest version; older releases are not backported. Upgrade before
reporting.

## Reporting a vulnerability

**Do not open a public issue.**

Use GitHub's private vulnerability reporting:
[Report a vulnerability](https://github.com/Streamline-HQ/streamline-opds-ext/security/advisories/new).
If that is unavailable to you, email dev@antoine-langlois.net instead.

Please include:

- The extension version and the Streamline version it runs on
- What an attacker can do: read another account's token, download a book
  without credentials, reach a file outside the library, etc.
- Steps to reproduce, ideally with the reader app and the request it sends
  (tokens redacted)

Expect an acknowledgement within a week and a fix or a decision within 30 days
for anything exploitable. This is a spare-time project, not a vendor with an
on-call rotation: if something is actively being exploited, say so up front and
I will prioritise it. I will credit you in the advisory and the release notes
unless you'd rather stay anonymous.

Please give me a chance to ship a fix before disclosing publicly. There is no
bug bounty.

## Scope

In scope: anything that breaks the security model of the extension on a
correctly configured Streamline:

- Authentication bypass on `/extensions/opds`: a feed, a download or a cover
  answered without a valid email and reader token, or to a locked account
- Reader tokens: one stored or logged in the clear, one that still works after
  it was rotated or turned off, one account reading or changing another's
- A download or cover reaching a file the book library does not hold
- SQL injection through a search or a feed parameter
- Supply-chain issues in the release pipeline: the published `opds.wasm` not
  matching its checksum or this repository's source

Out of scope:

- Vulnerabilities in Streamline itself (the extension host, its sandbox, its
  views): report those to
  [Streamline](https://github.com/DataHearth/streamline/security/advisories/new).
- OPDS over plain HTTP. Basic authentication sends the token with every
  request; Streamline is built to sit behind TLS.
- Every account seeing the whole book library. The catalogue is the library,
  not a per-user shelf.
- Vulnerabilities in reader apps (KOReader, Moon+ Reader, Librera, Marvin,
  Foliate). Report those upstream.
- Scanner or agent output with no demonstrated path to exploitation.

# Contributing

Thanks for taking an interest. Bug reports and small focused PRs are the most
useful things you can send.

## Before you write code

- **Bug fix**: just open a PR. Link the issue if there is one.
- **New feature**: open an issue first so we can agree on the shape before you
  spend an evening on it. The catalogue is ebooks only, on purpose.
- **A change Streamline has to make** (a view column, an SDK function, how
  extensions are served) belongs in
  [Streamline](https://github.com/DataHearth/streamline), not here.

## Setup

Requires Go (the `go` directive in `go.mod`) and [Task](https://taskfile.dev).
Running the end-to-end suite also needs a Streamline binary built from a
version that serves extension contract 2.

```bash
git clone https://github.com/Streamline-HQ/streamline-opds-ext.git
cd streamline-opds
task build              # opds.wasm
```

The extension SDK is the `github.com/datahearth/streamline/extensions/sdk`
module, pinned in `go.mod` to a Streamline commit. To build against a local
Streamline checkout instead, use a `go.work` that is not committed:
`go work init . ./e2e && go work use ../streamline/extensions/sdk`.

## The loop

```bash
task lint                                  # golangci-lint: the guest for wasip1, e2e/ for the host
task fmt
STREAMLINE_BIN=../streamline/streamline task test
```

`task test` boots a real Streamline with `opds.wasm` configured as an
extension, in a scratch directory and on a free port, and drives
`/extensions/opds` over HTTP. CI runs `task build` and `task lint`.

## Conventions

- **One Go module for the guest**, built for `wasip1`, importing nothing but
  the SDK. The end-to-end suite is its own module (`e2e/`), with its own tools.
- **The host contract** (views, folders, services, mounts, cards) is described
  in Streamline's
  [Writing an extension](https://github.com/DataHearth/streamline/wiki/Writing-an-extension)
  page and in `extensions/sdk`. A guest keeps nothing in memory between
  requests: everything it needs again is a row in its own database.
- **Every link starts with `Request.BasePath`**, so the catalogue works under
  whatever path Streamline serves it from.
- **Tests**: Ginkgo + Gomega, a `Label("e2e")` on every `Describe`. Each spec
  uses its own accounts and cleans up after itself.
- **Comments**: only where the code cannot explain itself.
- **Releases**: the `Version` in `main.go` must equal the tag; see
  [Releasing](README.md#releasing).

## AI-assisted contributions

Streamline is built with heavy AI assistance, so this is not a "no AI" project,
and you don't need to hide that you used a coding agent. There is one rule:

**You are the author. The model is a tool.**

In practice:

- **Understand every line you submit.** If a reviewer asks why a function is
  shaped that way and the honest answer is "the model wrote it", the PR isn't
  ready. Read the diff before you open it.
- **Run `task lint` and the tests yourself.** An agent claiming the tests pass
  is not evidence that they pass. Paste real output if a change is subtle.
- **Verify the code against reality, not plausibility.** Agents invent SDK
  functions, view columns and API fields that look exactly right and don't
  exist. Build it.
- **Mention it in the PR description.** One line, "written with Claude Code",
  so a reviewer knows which failure modes to look for. No stigma attached.

What gets closed without much discussion:

- Large unreviewed diffs the submitter can't explain, or PRs that clearly
  weren't run
- Agent-generated refactors, "modernisations", or comment/docstring sweeps nobody
  asked for
- Scanner or agent output filed as a bug report or security report with no
  reproduction: see [SECURITY.md](SECURITY.md) for what a real report looks like

Reviewer time is the scarce resource here. A small PR you fully understand is
worth more than a large one you don't.

## Commits and PRs

Conventional Commits, subject line only unless the body earns its place:

```
fix: refuse a locked account
feat!: serve under /extensions/opds with contract version 2, links from the base path
```

`type(scope): msg`, scope optional. Keep the PR to one topic. Rebase on `main`
rather than merging it in. Fill in the PR template checklist.

## License

Contributions are licensed under [GPL-3.0-or-later](LICENSE), same as the
project.

# Contributing

**English** | [Русский](CONTRIBUTING.md)

Code of conduct: [CODE_OF_CONDUCT.en.md](CODE_OF_CONDUCT.en.md). Report a vulnerability privately, as described in [SECURITY.en.md](SECURITY.en.md), not in an issue. Questions and ideas go to Discussions.

## What you need

- macOS with the Xcode Command Line Tools (`xcode-select --install`), Go 1.27 (the version is in `go.mod`) and a network: `make` downloads the linter and other tools itself, except `shellcheck` (`brew install shellcheck`); `make test` also builds the release archive.

```sh
git clone https://github.com/Windemiatrix/photosync
cd photosync
make help   # all targets
make test   # every check before a PR
```

## How a change gets in

The path is the same for everyone: the maintainer, external contributors (from a fork branch) and Dependabot. AI agents acting on the maintainer's behalf take the same path and additionally follow [AGENTS.en.md](AGENTS.en.md).

1. Branch from an up-to-date `main`: `<type>/<topic>`, for example `feat/schema-photos12`, `fix/rest-retry`. Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`.
2. Test first, then code (TDD): a test that fails for the right reason, then the code that makes it pass. A bug fix starts with a test that reproduces the bug.
3. Before the PR, `make test` is green locally (on macOS): it runs every check, the linters included.
4. Open a PR into `main`. The description says what changes and why. There are no direct pushes to `main`.
5. `ci-ok` is green: it aggregates all required checks into one.
6. Only squash merge, done by the maintainer. No approvals are required for now because there is a single developer; merging stays the maintainer's decision.

## PR title

The PR title becomes the commit title on `main`, and the release-notes change list is built from these titles.

- Format `<type>(<scope>)[!]: <description>`, for example `feat(schema): add photos12 schema`. The scope is required: lowercase letters, digits, `._/-`.
- An incompatible change (commands, flags, config keys, the state schema) is marked with `!` right after the scope: `feat(config)!: rename remotes key`. It is the only marker: it decides the next version (see "Releases") and puts the title into the "Breaking changes" group of the release notes. `BREAKING CHANGE:` in the body is not recognized: the squash commit body is empty. Without a scope (`feat!: …`) the title is rejected.
- English, printable ASCII only, at most 70 characters, no `#<number>`: GitHub appends ` (#<PR number>)` itself.
- The `pr-title` workflow checks it when the PR is opened, on every push and on every title edit. Before opening the PR you can check the title locally: `make pr-title PR_TITLE='...'`.
- **Do not edit the title or body in the squash-merge dialog.** The only place to fix a title is the PR itself; otherwise `main` gets a title the check never saw.
- Commit titles inside a branch are not checked: squash discards them.

## A failed release on `main`

The history of `main` is never rewritten: force-push and deletion are forbidden, tags are immutable. A failed `release` job on `main` is fixed by the next PR.

## Dependencies

A new direct dependency goes in its own PR with a rationale: why it is needed, why the standard library is not enough, its license (compatible with MIT) and how it is maintained. The maintainer reviews `go.mod` and `go.sum`.

## Language

- Code, comments, commit messages and PR titles are in English.
- Documents are in Russian and English at the same time, with the same meaning: `*.md` and `*.en.md`.
- Command and output examples in documents are copied from `make help-output`. A new command, flag or error case joins the list of that target in the `Makefile`.

## Personal data

Public history cannot be undone: forks and archives keep whatever once reached the repository. Commits and PRs must not contain:

- Photos libraries, `*.sqlite*`, `state.db`, `runs/`, `*.jsonl`, run reports and journals;
- passwords, tokens, `client_secret`, token files (`~/.config/photosync/tokens/`);
- names of your albums, cloud paths, logins, UUIDs and GUIDs, exact counts from your library.

Tests and documents use generic examples: `IMG_0001.HEIC`, `2024/05-01 - Holidays`, `user@example.com`. Fixtures made from real materials are allowed only as the output of `tools/anonymize`: a leak scanner checks them, an automatic comparison with the source materials (`make fixtures-verify`) confirms that none of the input words got in, and the maintainer reads the summary of that comparison. Optional: run `gitleaks` and a personal list of forbidden strings, kept outside the repository, before committing.

## Tests that modify Photos

Tests tagged `photokit_live` import, change and delete objects in a real library. They run only manually, under a separate macOS user account with a separate Apple ID and a test library, never against your main library and never in CI. All other tests do not touch the library.

## External contributors

- CI for a PR from a fork runs after the maintainer approves it.
- Explain in the description any PR that changes `.github/`, `scripts/`, `Makefile`, `.goreleaser.yaml`, `.svu.yml`, `docs/release-notes-header*.md`, `go.mod` or `go.sum`: the maintainer checks that it does not weaken the checks. `.goreleaser.yaml` and the release-notes header run inside the release job with write access.
## Releases

A release comes out by itself after every merge of a PR into `main`. Once CI on that commit is green, the `release` job of `ci.yml` computes the version, builds the universal binary, signs it ad hoc, packs the archive, computes `checksums.txt`, publishes the GitHub Release (which creates the tag on that commit) and attests the provenance. The notes are built from PR titles; there is no `CHANGELOG.md`.

Nobody assigns a version: [svu](https://github.com/caarlos0/svu) computes it from the PR titles since the last tag, with the rules in `.svu.yml`. `make next-version` shows the next one.

| Titles since the last tag | before `1.0.0` | from `1.0.0` |
|---|---|---|
| there is `type(scope)!:` | minor | major |
| otherwise there is `feat(…)` | minor | minor |
| otherwise any others | patch | patch |

Version `1.0.0` is not inferred from titles: the maintainer releases it by running `ci.yml` by hand (Run workflow) with the `major` flag.

A published tag is never moved; a faulty version is superseded by the next one.

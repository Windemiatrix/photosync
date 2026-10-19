# photosync architecture

**English** | [Русский](architecture.md) | [← README](../README.en.md)

Contents: [Terms](#terms) · [1. Two deliverables](#1-two-deliverables) · [2. Components](#2-components) · [3. Data and state](#3-data-and-state) · [4. How synchronization works](#4-how-synchronization-works) · [5. Protection against data loss](#5-protection-against-data-loss) · [6. Testing](#6-testing) · [7. Build](#7-build) · [8. Release](#8-release) · [9. Installation and updates](#9-installation-and-updates)

A condensed overview of how the application is built. There is no code yet: this document describes the intended behavior. Points that depend on unconfirmed facts about macOS, PhotoKit, Yandex Disk and the build are marked **"stage 0"**: research done before implementation, which may change the decision. The `R-*` (plan rules) and `INV-*` (invariants) identifiers and the configuration keys (`targets.*`, `remote_priority`, `sync.*`, the `conflict` and `deletes` policies and others) are named here ahead of time: their full list will appear together with the code and a configuration reference.

## Terms

- **Storage** (remote): the cloud the tool works with, a WebDAV server or Yandex Disk. "Cloud" in the text means the same thing.
- **Item**: a Photos entry, that is a photo, a video, a Live Photo or a RAW pair.
- **Snapshot**: the state of the Photos catalog read into memory for the duration of a run.
- **Link**: a record that an item corresponds to a file in a storage (path, size, hashes).
- **Event**: a Photos album of the form `MM/DD Title` inside a year folder; it corresponds to the directory `MM-DD - Title`. **Thematic album**: any other album.
- **Quarantine**: the `.photosync/quarantine` directory in the cloud; instead of being deleted, a file is moved there.

## 1. Two deliverables

The [osxphotos](https://github.com/Windemiatrix/go-libs/tree/main/osxphotos) library knows nothing about synchronization: it reads a copy of `Photos.sqlite`, builds paths from templates and bridges to PhotoKit. The application reaches the library's I/O (`photosdb`, `photokit`) through a single adapter; the domain imports the pure `template` and `export` packages directly (section 2). The application depends on published library tags, and `replace` in `go.mod` is forbidden. Principle: of two solutions, take the simpler one until it is shown not to cope.

## 2. Components

```text
cmd ──► app ──► domain
cmd ──► adapters ──► domain      (app does not import adapters; adapters do not import app)
cmd, app ──► state, report, clock
cmd ──► config, i18n             (state, report, clock, config, i18n are concrete packages, no interfaces)
```

The code layout follows [bxcodec/go-clean-arch](https://github.com/bxcodec/go-clean-arch): `app` is the service layer, `adapters` the repository layer, `cmd` the delivery layer; interfaces are declared by their consumer, there is no separate interfaces package, and dependencies point inward, to `domain`.

- **`domain`**: pure rules with no I/O (the only external imports are `x/text` and the library's pure `template` and `export` packages): file names, dates, layout into folders and albums, matching, the plan. The plan is a pure function, so it is checked with table tests per `R-*` rule and with property tests.
- **`app`**: use cases (`init`, `doctor`, `plan`, `sync`, `conflicts`, `albums diff`, `undo`, `explain`, `relink`, `import-state`, ...) that drive I/O through their own interfaces; worker pools.
- **`adapters`**: implementations of the interfaces that `app` declares; they make no decisions.
- **Concrete packages** (no interfaces: tests use the real ones, including SQLite; `clock` has a controllable test implementation): `state` (SQLite), `report` (`json/md/csv`), `clock`, `config`, `i18n` (ru/en).

`app` interfaces: `PhotoLibrary` (snapshot through `photosdb`; download, import, albums and deletion through `photokit`); `Remote` (WebDAV with Yandex Disk specifics; a read-only Yandex REST adapter for a flat listing, planned after the core synchronization and only if stage 0 research shows that a repeat run cannot meet the target time without it); `SecretStore` (Keychain); `MediaProbe` (own EXIF/ISOBMFF/QuickTime parsing over Range requests); `Converter` (`ffmpeg`); `SpaceManager`, `NetworkMonitor`; `UI` (no full-screen interface). A linter enforces layer boundaries. Every cgo adapter has a stub for other platforms, so the code builds and tests run on Linux too.

Main commands:

| Command | What it does |
|---|---|
| `init` | setup wizard: storages, policies, passwords into the Keychain |
| `doctor` | checks the environment, permissions and storage reachability |
| `status` | shows the current synchronization state |
| `plan` | builds and shows the plan without changing any data |
| `sync` | builds the plan and, with `--apply`, carries it out |
| `conflicts` | lists open conflicts; `conflicts resolve` records a decision |
| `albums diff` | differences between Photos albums and cloud directories |
| `undo` | rolls back the last applied run (with `--apply`) |
| `explain` | shows which `R-*` rule produced a decision |
| `relink` | manually fixes the link between an item and a file |
| `import-state` | loads links from the materials of a previous manual sync; with `--apply` it writes them to the state, and the plan then contains no transfers for files that are already linked |

Utility commands (configuration check, viewing storages and the log, version) are not listed here: their list will appear with the reference.

## 3. Data and state

`state_dir` holds `state.db` (SQLite), `lock`, `tmp/` and per-run reports in `runs/<run_id>/` (plan, journal, result; kept for 90 days). Migrations only go forward; a copy `state.db.bak-<version>` is made before one, and a database newer than the binary is refused. Configuration lives in `~/.config/photosync/config.yaml`, secrets in the Keychain.

- `assets`, `asset_resources`, `asset_history`: the last applied snapshot of Photos and the previous identifiers after replacements.
- `albums`, `asset_albums`, `album_map`: albums, folders, membership, and "album or folder <-> directory" pairs that are never recomputed.
- **`links` is the base of the three-way comparison:** for an "item, storage" pair it holds the path, size, hashes, `etag`, role, match level and status (`active`, `quarantined_*`, `detached`).
- `conflicts`, `runs`, `journal` (with prior values for undo), `probe_cache`.

Only five commands change the registry of items and links: `sync --apply`, `undo --apply`, `import-state --apply`, `relink` and `conflicts resolve`. The cloud scan is not stored: it lives in memory for one run. `plan`, `status`, `explain`, `doctor` and `albums diff` write only a `runs` row, report files and the cloud-metadata cache `probe_cache`. Whatever is derived from the snapshot is written in phase 0 of `sync --apply` in a single transaction (the phases are described in section 4). An item is identified by its cloud GUID (whether it is unique and stable is for stage 0 to establish; if not, the key becomes the master GUID plus the original's fingerprint), then its UUID, then the replacement history; an album by its cloud GUID, `PHCloudIdentifier`, or path plus title (which of these work is also for stage 0 to establish).

## 4. How synchronization works

Cloud layout: `Year/MM-DD - Title` for events, `Year/MM-DD` for items without an event. The event format is fixed; the "untitled" text comes from the `sync.untitled_event` key written by `init`. Other (thematic) albums are synchronized where an item has a link in the album's directory. Multi-valued template fields yield a single value (the primary placement). Changing `path_template` affects new files only. The iCloud Shared Library and the personal library are laid out under different storage roots (`targets.shared` and `targets.personal`). Importing directly into the Shared Library is unconfirmed (stage 0): if it does not work, the item lands in the personal library and waits for a manual move based on the report.

A single `plan` or `sync` run has seven steps:

1. **Photos snapshot** in memory. Shared albums (not to be confused with the Shared Library) and unsaved items from "Shared with You" are dropped; hidden items and non-selected burst frames follow policies; non-selected frames go to the event folder of the burst's key frame. Whether PhotoKit can fetch hidden items and non-selected burst frames is for stage 0 to confirm.
2. **Cloud scan** across all storages. Any traversal error is fatal: a partial scan is never read as "files were deleted". Files younger than `sync.settle_minutes` minutes are left alone. Directories listed in `sync.edited_dirs` and files matching `*_edited.*` count as edits and follow the `edited_files` policy.
3. **Enrichment:** dates of unlinked files are read via Range requests; if the server provides no hash, the file is downloaded and hashed.
4. **Matching new items.** Levels from strong to weak: `exact`, `same_shot`, `same_shot_weak`, `same_time_size`, `probable`, `pair_component` (a Live or RAW pair component). Assignment is one-to-one, greedy, with a total order of candidates; a name alone never creates a link. A pair with different content (a recompressed copy, an export to another format) becomes a `derived_copy`: the better file has more pixels, then RAW over non-RAW, then the larger size in bytes.
5. **Three-way comparison** of base, Photos and cloud. Identical changes on both sides are not a conflict, and "modified versus deleted" is resolved in favor of the modification. Changes from all storages are merged into one target state for Photos (when storages disagree, the one with higher priority in `remote_priority` wins), then operations are built per storage. The `conflict` policy resolves conflicts (`remote_wins` is recommended; `ask` and `skip` exclude the item and give exit code 2). Rules carry `R-*` identifiers for `explain`.
6. **Report:** a summary, plan files, `plan_digest` in `runs`. The snapshot and links are not modified.
7. **Apply** (`--apply` only): the plan is rebuilt and compared by `plan_digest` with the one that was reviewed. It runs in phases 0-6.

Apply phases: **0** write the snapshot and links, `owner.json` in the cloud; **1** directories, moves, copies, quarantine, deleting files and empty directories in the cloud (under `deletes: propagate`), folders and albums in Photos; **2** Photos to cloud (original to disk, hash, atomic upload to each storage); **3** cloud to Photos (download, parse, convert if needed, import with albums; the MP4 from `convert_mp4`, which converts unsupported video formats to MP4 with `ffmpeg`, also goes to the other storages); **4** album membership, item updates; **5** deleting Photos items in one batch through the macOS system dialog (whether it appears from a console program and whether deletion goes through as one batch is for stage 0 to establish), replacements; **6** cleanup.

**First sync** of populated libraries: album pairs are matched by name, then by contents, and the rest are created; membership differences are settled by the `conflict` policy, and `photosync albums diff` shows the outcome before anything is applied. Files already in the cloud are linked to items rather than uploaded again.

Retries: up to 6 attempts with exponential backoff; after three consecutive failed operations a storage is skipped for that run. Exit codes: 0 success, 1 error or refusal, 2 open conflicts, 3 out of space, 130 Ctrl+C.

## 5. Protection against data loss

- **`--yes` is not accepted** (without a terminal: exit code 1 naming what is required, a flag or a terminal) when:
  - there is no reviewed plan or it changed: manual confirmation in a terminal is needed;
  - the plan has irreversible operations (cloud deletion under `deletes: propagate`): you must type their number in a terminal;
  - deletions, quarantines and replacements exceed the threshold (50 items or 5%): `--allow-mass-changes` or typing the number is required;
  - on a first sync more than 1% of pairs are probable matches (`same_time_size`, `probable`): you must review a sample and confirm separately, or pass `--confirm-probable`.
- **Deletions.** The `deletes` policy is mandatory and has no default: `none` (do not propagate), `quarantine` (recommended) or `propagate`. Cloud quarantine is `.photosync/quarantine/<date>`, kept 90 days by default (`sync.quarantine_keep_days`). Under `propagate`, a cloud deletion is irreversible as far as photosync is concerned; whether the file lands in the Yandex Disk trash is for stage 0 to establish. A deletion in Photos goes to Recently Deleted and is confirmed by the system dialog.
- **Cloud.** `MOVE` and `COPY` always send `Overwrite: F`, except for moves into quarantine inside `.photosync/`. Before deleting, quarantining or replacing, size and `etag` are checked against the link record in `links`. Directories outside `.photosync/` are never deleted recursively, and the storage root is untouchable. Paths from the cloud are untrusted: `path.Clean`, prefix check, NFC. Uploads are atomic: write to `.photosync/tmp`, then `MOVE` without overwrite.
- **Unconfirmed (stage 0):** Yandex Disk guarantees (`Overwrite: F` with a 412 response, atomic writes through `MOVE`, `etag` as md5, the fate of `DELETE`) and whether the system deletion dialog appears from a console program. If the answer is negative, the protection will be redesigned (for example `Stat` instead of an atomic precondition, or REST).
- **Undo.** `undo --apply` rolls back only the last applied run, strictly in order, from a journal of prior values; a target that changed after the run is skipped and reported. What cannot be undone is listed up front.
- **Resuming.** Operations are idempotent, a link is written right after each transfer, and before retrying an import the item is looked up by the traits recorded in the journal. A failure at any boundary cannot cause a double import.
- **One run, one Mac.** Mutating commands take the `state_dir/lock` lock; `<root>/.photosync/owner.json` ties the cloud to one state, and a foreign state is refused. Several Macs against one cloud are not supported. Apply refuses if the library is not the system one or not the recorded one.
- **Secrets, network, input.** Secrets (WebDAV passwords, OAuth tokens) live only in the Keychain and are typed without echo or passed through `--password-stdin`, never through arguments or environment variables. Only `https://` (`http://` is allowed solely for loopback addresses in tests; certificate checks cannot be disabled). Network connections go only to configured storages and, when REST access is enabled, to Yandex authorization addresses. Response sizes are bounded, `ffmpeg` runs with an allowlist of demuxers, and cloud names are escaped in the terminal and CSV.

## 6. Testing

- **Domain:** tables per `R-*` rule; property tests of `INV-*` invariants (after apply the plan is empty, order is deterministic, the last original never disappears, operations stay inside the root, no double import).
- **Matching:** a world generator with known ground truth; fixtures from anonymized materials with a human-verified result; before commit they pass a leak scanner and the maintainer reviews a sample.
- **`app` scenarios:** a real `state` package (SQLite); the WebDAV adapter against a test server with failures (429, 503, dropped connection, lost response); fakes for Photos, Keychain, disk space and network. A failure at any operation boundary must end in the same outcome as a run without it.
- **Contracts:** one set of checks for all `Remote` implementations (test server, recorded Yandex responses, the real Yandex by hand) and for `PhotoLibrary`.
- **Other:** parser fuzzing; ru and en key parity; with `--output json`, stdout is exactly one valid JSON document.
- **PhotoKit:** tests that modify the library (import, deletion, albums) run by hand and only under a separate macOS account with its own Apple ID and a test library; never in CI and never on a primary library.

Coverage: the domain at least 95% per package, the rest at least 80%, except for code that runs only against real system APIs (cgo adapters, stubs, `cmd/`, `tools/`); the list of exclusions lives in the check script.

## 7. Build

The cgo binary (PhotoKit, Keychain, network monitoring) is built on macOS only: `MACOSX_DEPLOYMENT_TARGET=13.0`, `-trimpath`. The arm64 and amd64 slices are merged with `lipo` into a universal binary, then a mandatory ad-hoc signature `codesign -s -` is applied (not an Apple signature and no notarization; no certificates needed); the archive and checksums are computed after it. The `Info.plist` with the Photos usage description is embedded through a directive in `cmd/photosync`. The build is identical locally and in CI (`make release`); bit-for-bit reproducibility is not promised, and provenance is covered by an attestation.

**Unconfirmed (stage 0):** cross-compiling cgo for amd64 on an arm64 runner; compatibility of the ad-hoc signature with the hardened runtime, cgo and an embedded plist; whether cgo allows the plist-embedding flag; running on macOS 13.

## 8. Release

**Branches and PRs.** Work happens on `<type>/<topic>` branches; every change reaches `main` through a pull request with no required approvals: the owner squash-merges it after green CI. The PR title becomes the `main` commit title (Conventional Commits, English, at most 70 characters; GitHub appends the PR number). Force-pushes to and deletion of `main` are forbidden, tags are immutable; outside contributors follow the same path.

Versions: pre-releases `v0.x.0-alpha.N` and `v0.x.0-beta.N`, then `v1.0.0`; a release's `go.mod` points at a published library tag, with no pseudo-version and no `replace`. The maintainer's script creates the tag after checks (clean `main`, green CI); the tag workflow re-verifies the commit, builds the release and creates a **draft** GitHub Release.

Contents: `photosync_<version>_darwin_universal.tar.gz` (the binary, `LICENSE`, README), `checksums.txt` and a build provenance attestation. There are no per-architecture archives, SBOM or Homebrew in the first version. Release notes carry verification instructions in two languages and a changelog built from PR titles (the squash commits between tags). Before publishing, a manual checklist runs on a clean account (arm64, and amd64 on Intel or under Rosetta; macOS 13 and the latest).

## 9. Installation and updates

Step-by-step instructions (download, verification, installation, updates, uninstall) live in the [README](../README.en.md#installation), and macOS permissions are described there under "Requirements (planned)". Additions to the README: the ad-hoc signature ties the Photos and Keychain permissions to the code hash, so macOS may ask for them again after an update (stage 0 confirms this); for `go install`, stage 0 must confirm a build without `replace` and with the embedded plist.

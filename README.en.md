# photosync

**English** | [Русский](README.md)

[![CI](https://github.com/Windemiatrix/photosync/actions/workflows/ci.yml/badge.svg)](https://github.com/Windemiatrix/photosync/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Your Mac Photos library as plain folders on Yandex Disk, and back.**

For people who keep their photos in Photos on a Mac (iCloud Photos included) and want a second copy they can open without Apple: in a browser, or in the Disk app on Windows or Android. photosync is a macOS command-line tool. It will lay photos and videos out as folders such as `2024/05-01 - Holidays/IMG_0001.HEIC` and carry new items both ways: new items from the Mac go to the Disk, items added to the Disk go into Photos. Files already on the Disk are not uploaded again. The tool shows a plan first and deletes nothing without your consent.

**Early development: sync does not work yet** (see Status).

## Status

**Work in progress, not usable yet.** There is no sync, and the tool does not touch your photos. `v0.x` releases come out automatically after every merge, but they hold only a skeleton: one command works, `photosync version`. Everything below describes the intended behavior.

Already in place:
- research into the behavior of macOS, Photos and Yandex Disk, on which the design is built;
- the program skeleton and automatic releases: an archive for Mac, checksums and a provenance attestation;
- CI on macOS: tests with the race detector, a coverage threshold, a linter;
- in the [osxphotos](https://github.com/Windemiatrix/go-libs/tree/main/osxphotos) library: detecting the Photos schema and reading photos from a copy of the Photos database (`photosdb.Open`); albums and filters come next.

Next: reading the Photos library → scanning the Disk → the plan → upload and import → the first working version.

Questions and ideas go to [Discussions](https://github.com/Windemiatrix/photosync/discussions).

## How the tool protects your photos (as designed)

- **Plan first.** `photosync plan` shows what would happen and changes nothing. Changes happen only with `--apply`.
- **The Photos database is read from a copy**, read-only. The tool will change the library only through the system Photos interface.
- **Deletions are your choice.** You pick a policy during setup (`init`); until you do, the tool changes nothing. `none` — deletions are not carried over; `quarantine` (recommended) — a Disk file goes to `.photosync/quarantine` for 90 days, a Photos item to Recently Deleted; `propagate` — straight to the Disk trash. Deletions from Photos are confirmed in a macOS system dialog (one per batch).
- **A human confirms risky changes.** Before irreversible operations you type how many there are in the terminal; bulk changes (over 50 items or 5%) need the same or the `--allow-mass-changes` flag. `--yes` does not work here.
- **Undo.** Every run writes a journal; `photosync undo --apply` rolls back the last run, except irreversible operations, which the plan lists up front.
- **Originals are synchronized**, Live Photos and RAW pairs included.
- **Disk access and privacy.** The tool never sees your Yandex password: you sign in with a code at https://ya.ru/device. The app gets permission to read and write files on your Disk (`cloud_api:disk.read`, `cloud_api:disk.write`). Network access goes only to the Yandex Disk API, its file-transfer hosts and Yandex sign-in: no telemetry, update checks or crash reports. The token is a `0600` file in `~/.config/photosync/tokens/`; programs running under your account can read it. Revoke access with `photosync remote logout <name>` or in Yandex ID settings.
- **Open source (MIT).** Test data is anonymized and checked by a leak scanner; tests that modify Photos run only under a separate macOS test account.
- A repeated run with no changes does nothing; one Mac per cloud; the interface is in Russian and English.

Details: [Data-loss protection](docs/architecture.en.md#5-protection-against-data-loss).

## What you get on the Disk

```text
<your Disk folder>/
  2024/
    05-01 - Holidays/       ← the album "05/01 Holidays" in the "2024" folder in Photos
      IMG_0001.HEIC
    05-14/                  ← items without such an album go by date
      IMG_0002.JPG
```

An event album is an album of the form `MM/DD Title` inside a year folder in Photos; a file added to such a directory on the Disk goes into that album.

## What it will look like

The commands do not exist yet; this is the plan:

```text
photosync init           # setup: the Disk folder, Yandex sign-in by code, what to do with deletions
photosync plan           # what would be done — changes nothing
photosync sync --apply   # carry out the plan
```

The output of `plan` will appear together with the command: it will show what is going to be uploaded to the Disk, imported into Photos, moved and deleted, and the totals.

Scheduled runs use the standard macOS mechanism (a LaunchAgent); a ready-made configuration file will come with the first working version.

## Why

A library can be exported once (`osxphotos export`, the export from Photos, a data request to Apple), but the result is a one-way copy that is out of date at once. photosync keeps the folders on the Disk and Photos in agreement.

- **A copy a person can read.** Files sit on the Disk as plain folders and open without Photos.
- **No starting from scratch.** Photos already uploaded to the Disk by hand are matched with Photos and not uploaded again.
- **The personal and the Shared iCloud libraries** are laid out in different Disk folders.
- **The first storage is Yandex Disk;** the storage contract does not depend on it, so other clouds can be added later.

## How it differs

- **rclone, Syncthing** sync files but do not see the Photos library: albums, Live Photos and RAW pairs are just files inside an opaque package to them.
- **[osxphotos](https://github.com/RhetTbull/osxphotos)** can export from Photos and import into Photos, but each direction is a separate command; it does not merge the changes of two sides or carry deletions over.
- **photosync** compares three states (the previous run, Photos, the Disk), builds one plan for both directions with albums and deletions, and rolls back the last run.

## Not yet

- Shared albums are not synchronized.
- Importing directly into the Shared Library is not verified; if it does not work, the item lands in the personal library and the report flags it for a manual move.
- Several Macs against one cloud are not supported.
- There is no schedule command: a sample LaunchAgent will be provided.
- macOS and Yandex Disk only.

## Requirements (planned)

- A Mac with macOS 13 or newer, Apple Silicon or Intel. Running on macOS 13 is not verified yet.
- Access to Photos: macOS asks for it on first run. Full Disk Access may be needed too: to read the library database, the tool copies it and reads the copy read-only. It will change the library (import, albums, deletion) only through the system Photos interface. `photosync doctor` will show which permissions are missing.
- `ffmpeg` — only if you enable converting unsupported videos to MP4.

New macOS versions are supported once verified on them.

## Installation

There is no working version yet, so there is no point installing the `v0.x` releases. Every release is one archive for Macs (Apple Silicon and Intel); installation and verification (the checksum and `gh attestation verify`, the proof that the archive was built from this repository) are in the release notes.

Updates are manual too: the tool does not go online for updates by itself. How to remove the tool and its data: [docs/architecture.en.md](docs/architecture.en.md#9-installation-and-updates).

From source (macOS and Go 1.27 are required): `go build ./cmd/photosync`. A build of your own cannot sign in to Yandex: the application credentials are embedded only in release builds.

## How to help

- Tell us in [Discussions](https://github.com/Windemiatrix/photosync/discussions) about your use case: roughly how big your library is, your macOS version, what you use now. Do not post personal names or paths.
- Help with code is welcome already: checks on different macOS versions, Photos database schemas, documentation review. How to build and check: [CONTRIBUTING.en.md](CONTRIBUTING.en.md).

## Links

- [How it works](docs/architecture.en.md): components, the sync algorithm, data-loss protection, testing, build and release.
- [Contributing](CONTRIBUTING.en.md) · [Security](SECURITY.en.md) · [Discussions](https://github.com/Windemiatrix/photosync/discussions)
- Reading Photos (PhotoKit is planned): [go-libs/osxphotos](https://github.com/Windemiatrix/go-libs/tree/main/osxphotos), a Go port of parts of the Python project [osxphotos](https://github.com/RhetTbull/osxphotos) (Rhet Turnbull, MIT).

## License

[MIT](LICENSE), © 2026 Windemiatrix.

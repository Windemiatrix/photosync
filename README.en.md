# photosync

**English** | [Русский](README.md)

A macOS command-line tool that synchronizes the Photos library (iCloud Photos) with cloud storage; the first version supports Yandex Disk through its REST API. Photos and videos sit in the cloud as plain folders (`Year/MM-DD - Title`), and Photos albums map to directories. Changes flow both ways: new items from the Mac are uploaded, and new items from the cloud are imported into Photos.

## Status

**Design phase. There are no releases, and the sync code is not written yet.** The architecture is written down; research into the behavior of macOS, PhotoKit and Yandex Disk is complete; the remaining unknowns (importing into the Shared Library, stability of cloud identifiers) depend on iCloud and are checked during implementation. Everything below describes the intended behavior, not a finished program.

## Why

- Keep a second, human-readable copy of your library in the cloud: it opens in any file manager without Photos.
- Avoid lock-in to a single service: the same files are available from any device and any program.
- Support both the iCloud Shared Library and the personal one. Importing directly into the Shared Library is not yet confirmed (needs checking with iCloud); if it does not work, the item lands in the personal library and you move it by hand using the report. Shared albums are not synchronized.
- Replace manual syncing: the tool matches files already sitting in the cloud with Photos items and does not upload them again.

## What it will look like

The planned workflow (the commands do not exist yet):

```text
photosync init                  # setup wizard: storages, Yandex sign-in by code, policies
photosync remote auth <name>    # sign in to Yandex Disk: a code for https://ya.ru/device
photosync remote logout <name>  # delete the token file and revoke access
photosync doctor                # check the environment, permissions and storage reachability
photosync plan                  # what would be done; changes nothing
photosync sync --apply          # carry out the plan
```

There is no schedule command in the first version: for scheduled runs this README will include a sample LaunchAgent plist.

File transfers in both directions run in parallel: 256 by default; for a slow link, lower `limits.parallel` in the settings (the cloud scan is not bounded by it).

## Key principles

- **Plan first, act second.** `photosync plan` changes no data and shows what would happen. Anything that changes data runs only with `--apply`.
- **Nothing is lost without explicit consent.** The `deletes` policy is mandatory and has no default: without it, commands that change data refuse to run. The options:
  - `none`: deletions are not propagated to the other side;
  - `quarantine` (recommended): an item deleted on one side goes to quarantine in the cloud (`.photosync/quarantine`, 90 days by default) or to Recently Deleted in Photos; after the period the cloud file is deleted permanently, bypassing the trash;
  - `propagate`: the cloud file goes to the Yandex Disk trash; `undo` will not bring it back, you can restore it by hand until the trash is emptied, and until then it counts against the Disk quota.
- **Risky changes need a human.** Irreversible operations are confirmed by typing their number in a terminal; bulk changes need the `--allow-mass-changes` flag or typing the number; `--yes` is not accepted in either case.
- **Undo.** Every run writes a journal; `photosync undo --apply` rolls back only the last applied run, except for irreversible operations, which are listed up front.
- **A repeat run with no changes does nothing**: zero operations and zero bytes transferred. Exceptions: files that have only just appeared (they are left alone for a few minutes), open conflicts, and runs with `--limit`.
- **Privacy.** Tokens are stored in plain text in a file with mode `0600`: any program running under your account can read it. The network is used only for the Yandex Disk API, its file-transfer hosts and Yandex OAuth: no telemetry, no update checks, no crash reports. Local run reports are written to your own disk only.
- **Originals.** Originals are synchronized (including Live Photos and RAW pairs), not edited renditions.
- **One Mac per cloud.** Several Macs working against the same cloud directory are not supported.
- **Two languages.** The interface is available in Russian and English.

## Requirements (planned)

- macOS 13-27 (the Photos schema of each later macOS is added once verified), Apple Silicon and Intel, one universal binary (running on macOS 13 is not verified yet).
- The system Photos library and macOS permissions: access to Photos and possibly Full Disk Access (to read the Photos database). From a terminal, the Photos permission is effectively shared by everything launched from that terminal; a process started by a LaunchAgent gets its own permission.
- `ffmpeg` for converting unsupported video formats to MP4 (only if that policy is enabled).

## Installation

There are no releases yet. This is what installation will look like.

1. Download the archive and `checksums.txt` from the Releases page: `gh release download <tag> -R Windemiatrix/photosync -p 'photosync_*_darwin_universal.tar.gz' -p checksums.txt` (or `curl -LO`). Until `v1.0.0`, take a specific tag from the Releases page rather than `latest`.
2. Verify the hash: `grep darwin_universal checksums.txt | shasum -a 256 -c -`.
3. Verify provenance: `gh attestation verify photosync_<version>_darwin_universal.tar.gz --repo Windemiatrix/photosync` (exact flags, including `--signer-workflow` pointing at the release workflow, to be confirmed).
4. Unpack and put it on your `PATH`: `tar xzf ... && install -m 755 photosync ~/bin/`.
5. If the file was downloaded with a browser and macOS blocks it: `xattr -d com.apple.quarantine ~/bin/photosync` (for that file only, and only after verifying).

The checksum protects only against corruption in transit: it sits next to the archive. Authenticity (built from this repository by the release workflow) is confirmed only by `gh attestation verify`; without `gh` authenticity cannot be verified.

The binary carries only an ad-hoc signature: it is not signed by Apple and not notarized, so macOS may block it, and a process started by a LaunchAgent may ask for Photos access again after an update (the permission is kept when run from a terminal). `photosync doctor` and the release notes warn about it.

Installing with `go install` is not recommended: such a binary is signed by the Go linker with the name `a.out`, and macOS shows "a.out" in the access prompt and in settings. The release binary is signed with the identifier `io.github.windemiatrix.photosync`. The photosync OAuth application credentials are embedded only in release builds: a build without them (`go install`, a local build) cannot sign in to Yandex and says so.

**Updates** are manual, through the same channels: there is no self-update and no network version check. A binary older than the state database refuses to run; rolling the binary back is possible while no migration has been applied, or from the database backup copy.

**Uninstall.** First run `photosync remote logout <name>` for each storage: it deletes the token file `~/.config/photosync/tokens/<name>.json` and revokes access at Yandex. Then these are removed separately:

- the binary;
- the state directory `state_dir` (by default `~/Library/Application Support/photosync`) and the configuration `~/.config/photosync/`;
- in the cloud: the `.photosync/` directory (quarantine and `owner.json`), if the cloud is no longer synchronized. While `owner.json` is in place, another photosync installation (with a different `state_dir`) cannot work with that cloud.

## Documents

- [Architecture](docs/architecture.en.md): components, data, the sync algorithm, data-loss protection, testing, build and release.
- The library for reading Photos and using PhotoKit: [Windemiatrix/go-libs, osxphotos module](https://github.com/Windemiatrix/go-libs/tree/main/osxphotos).

## License

[MIT](LICENSE), (c) 2026 Windemiatrix.

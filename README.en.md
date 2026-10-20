# photosync

**English** | [Русский](README.md)

A macOS command-line tool that synchronizes the Photos library (iCloud Photos) with cloud storage over WebDAV, including Yandex Disk. Photos and videos sit in the cloud as plain folders (`Year/MM-DD - Title`), and Photos albums map to directories. Changes flow both ways: new items from the Mac are uploaded, and new items from the cloud are imported into Photos.

## Status

**Design phase. There is no code and there are no releases yet.** The architecture is written down; several assumptions about macOS, PhotoKit, Yandex Disk and the build still have to be confirmed by research (the documents mark them as "stage 0"). Everything below describes the intended behavior, not a finished program.

## Why

- Keep a second, human-readable copy of your library in the cloud: it opens in any file manager without Photos.
- Avoid lock-in to a single service: the same files are available from any device and any program.
- Support both the iCloud Shared Library and the personal one. Importing directly into the Shared Library is unconfirmed (stage 0); if it does not work, the item lands in the personal library and you move it by hand using the report. Shared albums are not synchronized.
- Replace manual syncing: the tool matches files already sitting in the cloud with Photos items and does not upload them again.

## What it will look like

The planned workflow (the commands do not exist yet):

```text
photosync init           # setup wizard: storages, policies, passwords into the Keychain
photosync doctor         # check the environment, permissions and storage reachability
photosync plan           # what would be done; changes nothing
photosync sync --apply   # carry out the plan
```

## Key principles

- **Plan first, act second.** `photosync plan` changes no data and shows what would happen. Anything that changes data runs only with `--apply`.
- **Nothing is lost without explicit consent.** The `deletes` policy is mandatory and has no default: without it, commands that change data refuse to run. The options:
  - `none`: deletions are not propagated to the other side;
  - `quarantine` (recommended): an item deleted on one side goes to quarantine in the cloud (`.photosync/quarantine`, 90 days by default) or to Recently Deleted in Photos;
  - `propagate`: the cloud file is deleted irreversibly (as far as photosync is concerned). Whether a deleted file goes to the Yandex Disk trash is for stage 0 to establish.
- **Risky changes need a human.** Irreversible operations are confirmed by typing their number in a terminal; bulk changes need the `--allow-mass-changes` flag or typing the number; `--yes` is not accepted in either case.
- **Undo.** Every run writes a journal; `photosync undo --apply` rolls back only the last applied run, except for irreversible operations, which are listed up front.
- **A repeat run with no changes does nothing**: zero operations and zero bytes transferred. Exceptions: files that have only just appeared (they are left alone for a few minutes), open conflicts, and runs with `--limit`.
- **Privacy.** Passwords and tokens live in the Keychain. The tool talks only to the storages you configured (and to Yandex authorization addresses when access through the Yandex Disk REST API is enabled): no telemetry, no update checks, no crash reports. Local run reports are written to your own disk only.
- **Originals.** Originals are synchronized (including Live Photos and RAW pairs), not edited renditions.
- **One Mac per cloud.** Several Macs working against the same cloud directory are not supported.
- **Two languages.** The interface is available in Russian and English.

## Requirements (planned)

- macOS 13-27 (the Photos schema of each later macOS is added once verified), Apple Silicon and Intel, one universal binary (building for Intel and running on macOS 13 are to be confirmed by stage 0).
- The system Photos library and macOS permissions: access to Photos and possibly Full Disk Access (to read the Photos database). Granting them to the binary itself is preferred. If stage 0 does not confirm that, the permissions go to Terminal, and with them to every program launched from it.
- `ffmpeg` for converting unsupported video formats to MP4 (only if that policy is enabled).

## Installation

There are no releases yet. This is what installation will look like.

1. Download the archive and `checksums.txt` from the Releases page: `gh release download <tag> -R Windemiatrix/photosync -p 'photosync_*_darwin_universal.tar.gz' -p checksums.txt` (or `curl -LO`). Until `v1.0.0`, use a specific tag: a pre-release does not count as the "latest release".
2. Verify the hash: `grep darwin_universal checksums.txt | shasum -a 256 -c -`.
3. Verify provenance: `gh attestation verify photosync_<version>_darwin_universal.tar.gz --repo Windemiatrix/photosync` (exact flags, including `--signer-workflow` pointing at the release workflow, to be confirmed).
4. Unpack and put it on your `PATH`: `tar xzf ... && install -m 755 photosync ~/bin/`.
5. If the file was downloaded with a browser and macOS blocks it: `xattr -d com.apple.quarantine ~/bin/photosync` (for that file only, and only after verifying).

The checksum protects only against corruption in transit: it sits next to the archive. Authenticity (built from this repository by the release workflow) is confirmed only by `gh attestation verify`; without `gh` authenticity cannot be verified.

The binary carries only an ad-hoc signature: it is not signed by Apple and not notarized, so macOS may block it, and after an update it may ask again for Photos and Keychain access. `photosync doctor` and the release notes warn about it.

Installing with `go install` is not recommended: such a binary is signed by the Go linker with the name `a.out`, and macOS shows "a.out" in the access prompt and in settings. The release binary is signed with the identifier `io.github.windemiatrix.photosync`.

**Updates** are manual, through the same channels: there is no self-update and no network version check. A binary older than the state database refuses to run; rolling the binary back is possible while no migration has been applied, or from the database backup copy.

**Uninstall.** These are removed separately:

- the binary;
- the state directory `state_dir` (by default `~/Library/Application Support/photosync`) and the configuration `~/.config/photosync/`;
- the Keychain entries of the `photosync` service;
- in the cloud: the `.photosync/` directory (quarantine and `owner.json`), if the cloud is no longer synchronized. While `owner.json` is in place, another photosync installation (with a different `state_dir`) cannot work with that cloud.

## Documents

- [Architecture](docs/architecture.en.md): components, data, the sync algorithm, data-loss protection, testing, build and release.
- The library for reading Photos and using PhotoKit: [Windemiatrix/go-libs, osxphotos module](https://github.com/Windemiatrix/go-libs/tree/main/osxphotos).

## License

[MIT](LICENSE), (c) 2026 Windemiatrix.

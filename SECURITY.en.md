# Security policy

**English** | [Русский](SECURITY.md)

## How to report a vulnerability

Use GitHub private reporting: the **Security** tab → **Report a vulnerability**, or <https://github.com/Windemiatrix/photosync/security/advisories/new>. Do not use a public issue, Discussions or a PR for vulnerabilities: the report is visible only to the maintainer.

Include:

- the version (`photosync version`), the macOS version and the architecture;
- what happens and what damage is possible (secret leak, escape from the storage root, data loss, running someone else's code);
- reproduction steps on **synthetic** data.

## What you must not attach

The report is stored by GitHub and may become public when the advisory is published. Do not attach:

- `Photos.sqlite` and any copy or export of a library, photos and videos;
- `plan.*`, `journal.jsonl`, `state.db`, the `runs/` directory and logs you have not reviewed: they contain file and album names and paths;
- `config.yaml` (it contains cloud paths), token files (`~/.config/photosync/tokens/`), OAuth tokens, `client_secret`;
- screenshots and text with your album names, paths and logins.

Instead, describe the situation in generic terms (`IMG_0001.HEIC`, `2024/05-01 - Holidays`) and attach the output of `photosync version` and `photosync doctor`, after reviewing it. If a secret did end up in a report, revoke it first (token: `photosync remote logout <name>`; if the token may have been an old one or the command did not revoke access, revoke the photosync app's access in your Yandex ID settings), then say so.

## Response times

The project is run by one person, so the times are targets: acknowledgement within 7 days, assessment and a fix plan within 30 days. The advisory is published after the fix is released; the reporter is credited if they wish.

## Supported versions

Fixes are released for the latest minor release (before `1.0.0`, for the latest released version). Older versions are not supported: please upgrade.

## Scope

In scope: leaks of secrets and personal data; escaping the storage root or bypassing the data-loss protections; parsing of untrusted data from the cloud and from files; unsafe installation and the supply chain (workflows, releases, dependencies).

Out of scope: vulnerabilities in macOS, Photos and Yandex Disk; an attacker with administrator rights on your Mac; reading the token file by programs running under your own account; the OAuth application credentials embedded in release builds, which are public by design and can be extracted from the binary (the secrets are your tokens); the behavior of settings you chose yourself (for example, `deletes: propagate`) when it is documented.

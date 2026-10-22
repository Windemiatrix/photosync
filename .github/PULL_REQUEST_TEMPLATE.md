## What problem does this solve?

<!-- The problem or the missing behavior, in a few sentences. Link the issue if there is one. -->

## How does it solve it?

<!-- The approach, and anything a reviewer should look at first. Mention alternatives you rejected. -->

## Checklist

- [ ] The PR title is `<type>(<scope>): <description>`: English, printable ASCII, at most 70 characters, no trailing period. It becomes the squash commit title and a line in the release notes, so fix it here, not in the merge dialog.
- [ ] An incompatible change (commands, flags, config keys, state schema) is marked with `!` after the scope (`feat(config)!: ...`): it decides the next version.
- [ ] Tests were written first, and `make test` passes locally.
- [ ] User-visible changes are documented in Russian and English, and messages exist in both locale catalogs.
- [ ] A new dependency is justified in this description: why the standard library is not enough, and its license (MIT, BSD, Apache-2.0 or ISC).
- [ ] Changes to `.github/`, `scripts/`, `Makefile`, `.goreleaser.yaml`, `docs/release-notes-header*.md` or `go.mod` do not weaken any CI or release check (the last two run inside the release job).
- [ ] No personal data: no real album, folder or file names, paths, UUIDs, coordinates, logins or secrets in code, tests, fixtures, logs or this description.

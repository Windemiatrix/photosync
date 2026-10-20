## What problem does this solve?

<!-- The problem or the missing behavior, in a few sentences. Link the issue if there is one. -->

## How does it solve it?

<!-- The approach, and anything a reviewer should look at first. Mention alternatives you rejected. -->

## Checklist

- [ ] The PR title is `<type>(<scope>): <description>`: English, printable ASCII, at most 70 characters, no trailing period. It becomes the squash commit title, so fix it here, not in the merge dialog.
- [ ] Tests were written first, and `make test lint` passes locally.
- [ ] User-visible changes are documented in Russian and English, and messages exist in both locale catalogs.
- [ ] A new dependency is justified in this description: why the standard library is not enough, and its license (MIT, BSD, Apache-2.0 or ISC).
- [ ] Changes to `.github/`, `scripts/`, `Makefile` or `go.mod` do not weaken any CI check.
- [ ] No personal data: no real album, folder or file names, paths, UUIDs, coordinates, logins or secrets in code, tests, fixtures, logs or this description.

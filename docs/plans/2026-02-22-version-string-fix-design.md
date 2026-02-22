# Fix Version String in Automated oastools Updates

**Date**: 2026-02-22
**Status**: Approved

## Problem

The `update-oastools.yml` GitHub Actions workflow uses `actions/checkout` with the default `fetch-depth: 1` (shallow clone). After squash-merging the dependency bump PR and syncing to `origin/main`, the workflow runs `git describe --tags --always` to compute the application version for deployment.

In a shallow clone, `git describe` cannot trace the commit history back from HEAD to the nearest tag. It falls back to `--always` mode, outputting only the short commit hash (e.g., `d656885`) instead of the full version string (e.g., `v1.2.1-6-gd656885`).

This worked initially because the first automated bump was only 1 commit past the last tag, within the shallow depth. As bumps accumulated, the distance exceeded depth=1 and the version string regressed.

## Solution

Add `fetch-depth: 0` to the `actions/checkout` step in `.github/workflows/update-oastools.yml`. This fetches the full commit history and tags, allowing `git describe` to produce the correct `<tag>-<N>-g<hash>` format.

## Change

Single line addition in `.github/workflows/update-oastools.yml`:

```yaml
- uses: actions/checkout@v6
  with:
    token: ${{ secrets.OASTOOLS_WEB_PAT }}
    fetch-depth: 0
```

## Trade-offs

- **Cost**: Full clone is slightly slower than shallow, but negligible for this repo size.
- **Alternatives considered**: `git fetch --unshallow --tags` (more fragile), manual version construction (over-engineered).

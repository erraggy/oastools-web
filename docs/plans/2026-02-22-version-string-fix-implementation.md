# Version String Fix Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix `git describe` in the `update-oastools.yml` workflow so deployed versions show the full semver+hash format instead of just a commit hash.

**Architecture:** Single-line change adding `fetch-depth: 0` to the `actions/checkout` step so the full git history is available when `git describe --tags --always` runs during the deploy step.

**Tech Stack:** GitHub Actions YAML

---

### Task 1: Add fetch-depth: 0 to checkout step

**Files:**
- Modify: `.github/workflows/update-oastools.yml:20-22`

**Step 1: Add `fetch-depth: 0` to the checkout step**

Change lines 20-22 from:

```yaml
      - uses: actions/checkout@v6
        with:
          token: ${{ secrets.OASTOOLS_WEB_PAT }}
```

To:

```yaml
      - uses: actions/checkout@v6
        with:
          token: ${{ secrets.OASTOOLS_WEB_PAT }}
          fetch-depth: 0
```

**Step 2: Run `make lint` to validate YAML**

Run: `make lint`
Expected: PASS (no yamllint errors on the workflow file)

**Step 3: Commit**

```bash
git add .github/workflows/update-oastools.yml
git commit -m "fix: use full clone in update workflow for correct version string

## Problem
The update-oastools workflow used a shallow clone (depth=1), causing
git describe to output only the short commit hash instead of the full
semver+hash format (e.g., d656885 instead of v1.2.1-6-gd656885).

## Fix
Set fetch-depth: 0 on the actions/checkout step so git describe can
trace back to the nearest tag."
```

### Task 2: Push and open PR

**Step 1: Push branch and create PR**

```bash
git push -u origin fix/version-string
gh pr create \
  --title "fix: use full clone in update workflow for correct version string" \
  --body "## Summary
- Add \`fetch-depth: 0\` to \`actions/checkout\` in \`update-oastools.yml\`
- Fixes version string regression where deployed builds show only the short commit hash instead of full \`v1.2.1-N-g<hash>\` format

## Root Cause
The workflow's shallow clone (depth=1) prevents \`git describe --tags --always\` from tracing back to the nearest tag. As automated oastools bumps accumulated past the shallow depth, the version string degraded to just the commit hash.

## Test plan
- [x] \`make lint\` passes (yamllint validates workflow file)
- [ ] Next oastools release triggers the workflow and produces a correct version string"
```

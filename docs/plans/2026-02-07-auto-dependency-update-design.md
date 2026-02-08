# Automated oastools Dependency Update

## Problem

When the oastools module publishes a new release, the oastools-web app must be manually updated: bump `go.mod`, run checks, push, create a PR, merge, tag, and deploy. This is tedious and delays users seeing new oastools features on the web app.

## Solution

Automate the full pipeline: oastools release triggers a dependency bump in oastools-web, validates it, merges it, and deploys to Cloud Run.

## End-to-End Flow

```text
oastools: tag push (v1.49.0)
  -> GoReleaser runs, publishes release
  -> New step: fires repository_dispatch to oastools-web
       payload: { "version": "v1.49.0" }

oastools-web: receives repository_dispatch
  -> Checks out main
  -> go get github.com/erraggy/oastools@v1.49.0 && go mod tidy
  -> make check (lint, test, build)
  -> If check passes:
       -> Creates branch: chore/bump-oastools-v1.49.0
       -> Commits go.mod + go.sum
       -> Creates PR
       -> Immediately admin-merges (gh pr merge --admin --squash)
  -> Authenticates with GCP via Workload Identity Federation
  -> Triggers Cloud Build (gcloud builds submit)
  -> Cloud Run receives the updated app
```

## Design Decisions

### Run `make check` before creating the PR, not after

The workflow validates the build before the PR is created. If checks fail, no PR is created and the workflow fails visibly in the Actions tab. The PR serves as an audit trail, not a review gate. This avoids the complexity of waiting for PR CI to complete.

### `repository_dispatch` for cross-repo triggering

Preferred over `workflow_dispatch` (which only works within the same repo) and over polling/cron (which adds latency and unnecessary runs). The oastools release workflow sends a dispatch event with the version in the payload.

### Fine-grained PAT over GitHub App

For a single-owner, 2-repo setup, a GitHub App is unnecessary overhead. A fine-grained PAT scoped to oastools-web with `contents: write` + `pull_requests: write` is sufficient.

### `gh pr merge --admin --squash` for merging

Branch protections require review approval, and the repo owner cannot self-review. Admin merge bypasses this, which is the same pattern used for manual merges. For an automated single-file dependency bump, this is appropriate.

### Workload Identity Federation for GCP auth

Industry-standard approach for GitHub Actions to GCP authentication. No service account keys to store or rotate. GitHub's OIDC token proves identity to GCP.

### No oastools-web version tag for dep bumps

Version tags are reserved for actual source changes to the web app. Automated dependency bumps deploy directly via `gcloud builds submit` without tagging.

## Files to Create/Modify

### oastools (1 file modified)

- `.github/workflows/release.yml` -- add `repository_dispatch` step after GoReleaser

### oastools-web (2 files modified/created)

- `.github/workflows/update-oastools.yml` -- new workflow (see below)
- `CLAUDE.md` -- fix incorrect deployment trigger documentation

## Workflow: update-oastools.yml

```yaml
name: Update oastools dependency

on:
  repository_dispatch:
    types: [oastools-release]

jobs:
  update:
    runs-on: ubuntu-latest
    timeout-minutes: 15
    permissions:
      contents: read
      id-token: write

    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v6
        with:
          go-version: "1.25"

      - name: Update oastools
        run: |
          VERSION="${{ github.event.client_payload.version }}"
          go get "github.com/erraggy/oastools@${VERSION}"
          go mod tidy

      - uses: actions/setup-node@v6
        with:
          node-version: "22"
          cache: "npm"
      - run: npm ci
      - run: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.7.2
      - run: pip install yamllint
      - run: make check

      - name: Create and merge PR
        env:
          GH_TOKEN: ${{ secrets.OASTOOLS_WEB_PAT }}
        run: |
          VERSION="${{ github.event.client_payload.version }}"
          BRANCH="chore/bump-oastools-${VERSION}"
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git checkout -b "${BRANCH}"
          git add go.mod go.sum
          git commit -m "chore: bump oastools to ${VERSION}"
          git push -u origin "${BRANCH}"
          gh pr create \
            --title "chore: bump oastools to ${VERSION}" \
            --body "Automated dependency update triggered by oastools ${VERSION} release."
          gh pr merge --admin --squash

      - uses: google-github-actions/auth@v2
        with:
          workload_identity_provider: "projects/PROJECT_NUM/locations/global/workloadIdentityPools/github-actions/providers/github"
          service_account: "github-actions@PROJECT_ID.iam.gserviceaccount.com"

      - name: Deploy via Cloud Build
        run: |
          gcloud builds submit \
            --config=cloudbuild.yaml \
            --region=us-central1 \
            --substitutions=SHORT_SHA=$(git rev-parse --short HEAD)
```

## Workflow: release.yml addition (oastools)

Add after the GoReleaser step:

```yaml
      - name: Trigger oastools-web dependency update
        if: success()
        run: |
          curl -X POST \
            -H "Accept: application/vnd.github+json" \
            -H "Authorization: Bearer ${{ secrets.OASTOOLS_WEB_PAT }}" \
            https://api.github.com/repos/erraggy/oastools-web/dispatches \
            -d "{\"event_type\":\"oastools-release\",\"client_payload\":{\"version\":\"${{ github.ref_name }}\"}}"
```

## One-Time Setup (CLI commands)

### 1. Create fine-grained PAT

Via GitHub.com: Settings > Developer settings > Fine-grained tokens > Generate new token

- Name: `oastools-web-automation`
- Expiration: choose based on preference (e.g. 1 year)
- Repository access: `erraggy/oastools-web` only
- Permissions: `Contents: Read and write`, `Pull requests: Read and write`

### 2. Store PAT as secret in both repos

```bash
gh secret set OASTOOLS_WEB_PAT --repo erraggy/oastools
gh secret set OASTOOLS_WEB_PAT --repo erraggy/oastools-web
```

### 3. Set up Workload Identity Federation

```bash
PROJECT_ID=$(gcloud config get-value project)
PROJECT_NUM=$(gcloud projects describe $PROJECT_ID --format='value(projectNumber)')

# Create workload identity pool
gcloud iam workload-identity-pools create github-actions \
    --location=global \
    --display-name="GitHub Actions"

# Create OIDC provider
gcloud iam workload-identity-pools providers create-oidc github \
    --location=global \
    --workload-identity-pool=github-actions \
    --issuer-uri="https://token.actions.githubusercontent.com" \
    --attribute-mapping="google.subject=assertion.sub,attribute.repository=assertion.repository" \
    --attribute-condition="assertion.repository=='erraggy/oastools-web'"

# Create service account
gcloud iam service-accounts create github-actions \
    --display-name="GitHub Actions"

# Grant Cloud Build and Cloud Run permissions
gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:github-actions@${PROJECT_ID}.iam.gserviceaccount.com" \
    --role="roles/cloudbuild.builds.editor"

gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:github-actions@${PROJECT_ID}.iam.gserviceaccount.com" \
    --role="roles/run.admin"

gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member="serviceAccount:github-actions@${PROJECT_ID}.iam.gserviceaccount.com" \
    --role="roles/iam.serviceAccountUser"

# Allow GitHub Actions to impersonate the service account
gcloud iam service-accounts add-iam-policy-binding \
    github-actions@${PROJECT_ID}.iam.gserviceaccount.com \
    --role="roles/iam.workloadIdentityUser" \
    --member="principalSet://iam.googleapis.com/projects/${PROJECT_NUM}/locations/global/workloadIdentityPools/github-actions/attribute.repository/erraggy/oastools-web"
```

### 4. Update workflow with real GCP values

Replace placeholders in `update-oastools.yml`:
- `PROJECT_NUM` with actual project number
- `PROJECT_ID` with actual project ID

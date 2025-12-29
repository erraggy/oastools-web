# Deployment Guide

## Overview

This document covers deploying the oastools-web application to Google Cloud Run, including initial setup, continuous deployment via Cloud Build, monitoring, and cost management.

## Prerequisites

The deployment requires a Google Cloud account with billing enabled (credit card on file, though the free tier should cover expected usage), the Google Cloud CLI (`gcloud`) installed and authenticated, and a GitHub account with the oastools-web repository.

## Initial Google Cloud Setup

### Create a New Project

Creating a dedicated project isolates the application's resources and billing from other Google Cloud usage.

```bash
# Create project
gcloud projects create oastools-web --name="OASTools Web"

# Set as current project
gcloud config set project oastools-web

# Enable required APIs
gcloud services enable \
    cloudbuild.googleapis.com \
    run.googleapis.com \
    containerregistry.googleapis.com \
    artifactregistry.googleapis.com
```

### Configure Billing Alerts

Setting billing alerts prevents unexpected charges. Navigate to the Google Cloud Console, select Billing from the navigation menu, choose Budgets & alerts, and create a budget with alerts at 50%, 90%, and 100% of a $1 monthly target. This conservative threshold provides early warning if usage unexpectedly increases.

### Create Artifact Registry Repository

Artifact Registry stores the container images built by Cloud Build.

```bash
gcloud artifacts repositories create oastools-web \
    --repository-format=docker \
    --location=us-central1 \
    --description="Container images for oastools-web"
```

## Dockerfile

The Dockerfile uses a multi-stage build to produce a minimal final image. The builder stage compiles the Go binary with all optimizations, and the final stage copies only the binary and static assets into a scratch-based image.

```dockerfile
# Build stage
FROM golang:1.24-alpine AS builder

# Install certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=${VERSION:-dev}" \
    -o server ./cmd/server

# Final stage
FROM scratch

# Copy certificates for HTTPS client calls (if needed)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary
COPY --from=builder /build/server /server

# Copy static assets and templates (embedded, but listed for clarity)
# These are embedded in the binary via embed.FS

# Cloud Run expects PORT environment variable
ENV PORT=8080

# Run as non-root (Cloud Run default UID)
USER 65534

ENTRYPOINT ["/server"]
```

## Cloud Build Configuration

The `cloudbuild.yaml` file defines the build and deployment pipeline. Cloud Build triggers on pushes to the main branch, builds the container image, pushes it to Artifact Registry, and deploys to Cloud Run.

```yaml
# cloudbuild.yaml
steps:
  # Build the container image
  - name: 'gcr.io/cloud-builders/docker'
    args:
      - 'build'
      - '--build-arg'
      - 'VERSION=$SHORT_SHA'
      - '-t'
      - 'us-central1-docker.pkg.dev/$PROJECT_ID/oastools-web/server:$SHORT_SHA'
      - '-t'
      - 'us-central1-docker.pkg.dev/$PROJECT_ID/oastools-web/server:latest'
      - '.'

  # Push to Artifact Registry
  - name: 'gcr.io/cloud-builders/docker'
    args:
      - 'push'
      - '--all-tags'
      - 'us-central1-docker.pkg.dev/$PROJECT_ID/oastools-web/server'

  # Deploy to Cloud Run
  - name: 'gcr.io/google.com/cloudsdktool/cloud-sdk'
    entrypoint: 'gcloud'
    args:
      - 'run'
      - 'deploy'
      - 'oastools-web'
      - '--image'
      - 'us-central1-docker.pkg.dev/$PROJECT_ID/oastools-web/server:$SHORT_SHA'
      - '--region'
      - 'us-central1'
      - '--platform'
      - 'managed'
      - '--allow-unauthenticated'
      - '--memory'
      - '512Mi'
      - '--cpu'
      - '1'
      - '--timeout'
      - '60s'
      - '--concurrency'
      - '80'
      - '--min-instances'
      - '0'
      - '--max-instances'
      - '2'

# Store images in Artifact Registry
images:
  - 'us-central1-docker.pkg.dev/$PROJECT_ID/oastools-web/server:$SHORT_SHA'
  - 'us-central1-docker.pkg.dev/$PROJECT_ID/oastools-web/server:latest'

# Build timeout
timeout: '600s'

# Build options
options:
  logging: CLOUD_LOGGING_ONLY
```

## Cloud Build Trigger Setup

Connect the GitHub repository to Cloud Build for automatic deployments.

```bash
# Connect repository (interactive - opens browser for GitHub OAuth)
gcloud builds triggers create github \
    --repo-name=oastools-web \
    --repo-owner=erraggy \
    --branch-pattern='^main$' \
    --build-config=cloudbuild.yaml \
    --name=deploy-main

# Grant Cloud Build permission to deploy to Cloud Run
gcloud projects add-iam-policy-binding oastools-web \
    --member="serviceAccount:$(gcloud projects describe oastools-web --format='value(projectNumber)')@cloudbuild.gserviceaccount.com" \
    --role="roles/run.admin"

gcloud iam service-accounts add-iam-policy-binding \
    $(gcloud projects describe oastools-web --format='value(projectNumber)')-compute@developer.gserviceaccount.com \
    --member="serviceAccount:$(gcloud projects describe oastools-web --format='value(projectNumber)')@cloudbuild.gserviceaccount.com" \
    --role="roles/iam.serviceAccountUser"
```

## Manual Deployment

For initial deployment or testing changes before merging to main, deploy manually.

```bash
# Build locally and push
docker build -t us-central1-docker.pkg.dev/oastools-web/oastools-web/server:manual .
docker push us-central1-docker.pkg.dev/oastools-web/oastools-web/server:manual

# Deploy
gcloud run deploy oastools-web \
    --image us-central1-docker.pkg.dev/oastools-web/oastools-web/server:manual \
    --region us-central1 \
    --platform managed \
    --allow-unauthenticated \
    --memory 512Mi \
    --cpu 1 \
    --timeout 60s \
    --concurrency 80 \
    --min-instances 0 \
    --max-instances 2
```

## Cloud Run Service Configuration

The Cloud Run service configuration balances cost efficiency with reasonable performance for demo traffic.

**Memory (512Mi)**: Sufficient for parsing and processing OpenAPI specifications up to 2MB. The oastools library is memory-efficient, and the Go runtime's garbage collector handles allocation patterns well within this limit.

**CPU (1)**: A single CPU provides adequate processing power. OpenAPI operations are primarily CPU-bound during parsing and validation, but complete quickly for typical specifications.

**Timeout (60s)**: Allows for processing larger specifications or complex operations like joining multiple files. The application's internal 30-second timeout for individual requests provides a tighter bound.

**Concurrency (80)**: Each container instance handles up to 80 concurrent requests. Go's goroutine-based concurrency model handles this efficiently. The application's semaphore-based concurrency limiter (10 concurrent operations) provides additional protection.

**Min Instances (0)**: Scale to zero when not in use. This is essential for staying within the free tier during periods of inactivity.

**Max Instances (2)**: Limits the maximum number of container instances to prevent unexpected scaling costs. Two instances can handle significant traffic while capping potential charges.

## Environment Variables

Configure the application via environment variables set in Cloud Run.

```bash
gcloud run services update oastools-web \
    --region us-central1 \
    --set-env-vars="LOG_LEVEL=info,RATE_LIMIT_RPM=10,MAX_FILE_SIZE=2097152"
```

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP port (set by Cloud Run) | 8080 |
| `LOG_LEVEL` | Logging verbosity (debug, info, warn, error) | info |
| `RATE_LIMIT_RPM` | Requests per minute per IP | 10 |
| `MAX_FILE_SIZE` | Maximum upload size in bytes | 2097152 |
| `REQUEST_TIMEOUT` | Request processing timeout | 30s |

## Custom Domain (Optional)

Map a custom domain to the Cloud Run service for a friendlier URL.

```bash
# Verify domain ownership (follow prompts)
gcloud domains verify oastools.dev

# Map domain to service
gcloud beta run domain-mappings create \
    --service oastools-web \
    --domain oastools.dev \
    --region us-central1
```

After creating the mapping, update DNS records as instructed by Google Cloud. The mapping provisions a managed SSL certificate automatically.

## Monitoring and Logging

### View Logs

Cloud Run logs integrate with Cloud Logging. View logs via the console or CLI.

```bash
# Stream logs
gcloud run services logs tail oastools-web --region us-central1

# Read recent logs
gcloud run services logs read oastools-web --region us-central1 --limit 100
```

### Create Log-Based Metrics

Create metrics for monitoring application health.

```bash
# Error rate metric
gcloud logging metrics create error_rate \
    --description="Rate of 5xx errors" \
    --log-filter='resource.type="cloud_run_revision" AND resource.labels.service_name="oastools-web" AND httpRequest.status>=500'

# Rate limit exceeded metric
gcloud logging metrics create rate_limit_exceeded \
    --description="Rate limit exceeded events" \
    --log-filter='resource.type="cloud_run_revision" AND resource.labels.service_name="oastools-web" AND jsonPayload.message="rate limit exceeded"'
```

### Create Alerts

Set up alerts for concerning conditions.

```bash
# Alert on high error rate (placeholder - use Cloud Console for full configuration)
gcloud alpha monitoring policies create \
    --display-name="High Error Rate" \
    --condition-display-name="Error rate > 10%" \
    --notification-channels="<channel-id>"
```

For detailed alert configuration, use the Cloud Console to create alerting policies based on the log-based metrics.

## Cost Management

### Free Tier Limits

Cloud Run's free tier includes 2 million requests per month, 360,000 GB-seconds of memory, 180,000 vCPU-seconds, and 1 GB of outbound network traffic to North America.

For expected demo traffic (tens to hundreds of requests per day), the application should remain well within these limits.

### Cost Estimation

Assuming 1,000 requests per day with average 2-second processing time and 512MB memory allocation, the monthly usage calculates to approximately 30,000 requests (well under 2M), 30,000 vCPU-seconds (well under 180,000), and 30,000 GB-seconds (well under 360,000). This usage pattern incurs zero charges.

### Budget Monitoring

The billing alert configured earlier provides notification if usage approaches the budget threshold. Additionally, view current usage in the Cloud Console under Billing, selecting Reports and filtering by the oastools-web project.

## Rollback Procedure

If a deployment introduces issues, roll back to the previous revision.

```bash
# List revisions
gcloud run revisions list --service oastools-web --region us-central1

# Route traffic to previous revision
gcloud run services update-traffic oastools-web \
    --region us-central1 \
    --to-revisions=oastools-web-00002-abc=100
```

## Security Considerations

### Service Account Permissions

The Cloud Run service uses the default Compute Engine service account. For production use, create a dedicated service account with minimal permissions.

```bash
# Create service account
gcloud iam service-accounts create oastools-web-sa \
    --display-name="OASTools Web Service Account"

# Deploy with service account
gcloud run services update oastools-web \
    --region us-central1 \
    --service-account=oastools-web-sa@oastools-web.iam.gserviceaccount.com
```

The application requires no Google Cloud API access, so the service account needs no IAM roles.

### Request Filtering

Cloud Run provides DDoS protection and TLS termination at the load balancer level. The application's rate limiting provides additional protection against abuse. For enhanced protection, consider enabling Cloud Armor (additional cost).

## Verification Checklist

After deployment, verify the following.

**Health endpoint returns 200**: `curl https://oastools-web-xxx-uc.a.run.app/health`

**Validate endpoint processes files**: Upload a sample specification via the web interface.

**Rate limiting functions**: Send multiple rapid requests and verify 429 responses after the limit.

**Logs appear in Cloud Logging**: Check the Logs Explorer in Cloud Console.

**Billing alerts are configured**: Verify in Budgets & alerts.

**Continuous deployment triggers**: Push a commit to main and verify automatic deployment.

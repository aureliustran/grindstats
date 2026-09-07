# AWS deployment (thesis demo)

**Status:** authoritative for how this project is deployed and how it reaches production.
**Companions:** [`backend.md`](backend.md) · [`frontend.md`](frontend.md) ·
[`shared-contract.md`](shared-contract.md)

> **This document describes a deliberately cheap deployment, not the blueprint's target
> architecture.** The blueprint specifies ECS Fargate + RDS + ElastiCache + OpenSearch +
> Amazon MQ. That shape costs roughly **$150–200/month** and belongs to roadmap Phase 10.
> Building it now is the deployment-side version of the mistake `backend.md` §1 warns about.

---

## 1. Target vs. now

| | Target (blueprint §17–19) | Now (roadmap Phase 1–2) |
|---|---|---|
| Compute | ECS Fargate, 10 services behind a gateway | **One Lambda function**, `provided.al2023`, running the whole Gin engine |
| Ingress | ALB + API Gateway | **CloudFront** → Lambda Function URL (no ALB, no API Gateway) |
| Static | S3 + CloudFront | S3 + CloudFront (unchanged) |
| Postgres | RDS Multi-AZ | **Neon** free tier, external |
| Redis | ElastiCache | **Upstash** free tier, external |
| Search / RAG | OpenSearch Service | **None yet.** When needed: `pgvector` in the same Neon database |
| Messaging | Amazon MQ (RabbitMQ) | **None yet** — direct function calls |
| CI/CD | CodePipeline | **GitHub Actions** + OIDC |
| Cost | ~$150–200/mo | **~$0.01/mo** |

The important property is that **the application code does not know which of these it is
running under.** The Lambda adapter wraps the same `*gin.Engine`; every handler, middleware
and route is identical to what `docker-compose.yml` runs locally. Moving to Fargate at
Phase 10 is a change of deploy target, not a rewrite. Nothing in `services/monolith/internal/`
may branch on "am I in Lambda" beyond the single entrypoint switch in §4.

---

## 2. Account constraints this plan is built around

Verified 2026-09-07 against account `260684397283`:

- **The legacy 12-month free tier has expired.** Billing → Free Tier shows the legacy
  usage-table view with no usage; Billing → Credits shows **$0.00 and no active credits**.
- Accounts created on/after **15 Jul 2025** receive $100 in free-tier credits instead of the
  12-month allowances. This account has none, so it predates that date and is now past 12 months.
- **Only always-free allowances remain.** Everything else bills at full rate against a card.

That single fact drives every choice below. The allowances this deployment lives inside — none
of which have a 12-month clock:

| Service | Always-free allowance | Consumed by a demo |
|---|---|---|
| Lambda | 1M requests + 400,000 GB-s / month | <1% |
| Lambda Function URL | no per-request charge | — |
| CloudFront | 1 TB egress, 10M requests, 1,000 invalidation paths / month | ~2.7 MB of assets |
| CloudWatch Logs | 5 GB ingestion / month | fine unless request bodies are logged |
| SSM Parameter Store (standard) | unlimited standard parameters | ~6 parameters |
| IAM, OIDC provider | free | — |
| S3 | **not free** — 5 GB tier was 12-month | 2.7 MB ≈ $0.0006/mo |

**Re-verify before assuming this document is current.** Free tier terms change; the two
console pages above are the check, and §9 records what to look at.

### Services deliberately not used, and why

| Not used | Reason |
|---|---|
| **API Gateway** | Its 1M-request tier is 12-month only and is gone. Lambda Function URLs have no per-request charge. |
| **Application Load Balancer** | ~$16/mo, no free tier. The most common way a "free tier" project quietly costs money. |
| **RDS / ElastiCache** | ~$12/mo each post-expiry. |
| **OpenSearch Service** | ~$25/mo for the smallest domain. Not needed until the RAG phase; `pgvector` first. |
| **CodePipeline** | $1 per active pipeline per month, no free tier. See §6. |
| **CodeBuild** | 100 min/mo free, smallest ARM instance only. Unnecessary when Actions is free. |
| **Secrets Manager** | $0.40 per secret per month. Parameter Store SecureString is free. |
| **Route 53** | $0.50/mo per hosted zone plus domain registration. The `*.cloudfront.net` name is enough for a thesis. |
| **NAT Gateway** | ~$32/mo. The Lambda is not in a VPC, so none is required. |

---

## 3. Architecture

```
                 ┌────────────────────────────────────────────┐
  reviewer ─────►│ CloudFront   dxxxxxxxxxxxxx.cloudfront.net │  TLS included, free
                 │   default  /*         → S3 origin (OAC)    │
                 │   behavior /api/v1/*  → Lambda URL (OAC)   │
                 └────────────────────────────────────────────┘
                          │                          │
                 ┌────────▼─────────┐   ┌────────────▼──────────────┐
                 │ S3 bucket        │   │ Lambda  provided.al2023   │
                 │ private, OAC     │   │ arm64, 512 MB, 10 s       │
                 │ SPA build ~2.7MB │   │ Gin engine via            │
                 └──────────────────┘   │ aws-lambda-go-api-proxy   │
                                        └──────┬──────────────┬─────┘
                                               │              │
                                   Neon Postgres        Upstash Redis
                                   0.5 GB, pooled       256 MB, 500k cmd/mo
```

Three properties worth stating explicitly, because each is load-bearing:

**One CloudFront distribution serves both the SPA and the API.** This is not a cosmetic
choice. It gives HTTPS on the API with no domain and no certificate management, and it makes
every API call **same-origin** — CORS never enters the picture, and the SPA ships
`VITE_API_BASE_URL=/api/v1` rather than an absolute URL that differs per environment.

**The Lambda Function URL is locked to CloudFront via OAC** (`AWS_IAM` auth type + an origin
access control). Without it the function URL is a second, unprotected public entrance that
bypasses the CDN entirely.

**Postgres and Redis are outside AWS.** There is no free AWS option for either post-expiry.
Neon's *pooled* connection string (PgBouncer) is mandatory, not optional — a Lambda that
opens a direct connection per invocation exhausts the connection limit under any concurrency.

### Accepted limitations

These are real and should be stated in the thesis rather than hidden:

- **Cold start.** Go on `provided.al2023` is ~100–300 ms. Neon's free tier auto-suspends when
  idle and takes a few hundred ms to wake. First request after a quiet period is ~1 s.
- **0.5 GB Postgres** and 256 MB Redis. Adequate for a demo dataset, not for real users.
- **No RabbitMQ, no Elasticsearch.** Neither is wired up in the repo yet (`backend.md` §1),
  so this is not a regression against current capability.
- **Datastore durability is a third party's free tier.** Never the only copy of demo data —
  keep a seed script in the repo and treat the hosted database as reproducible.

### If credits become available

If a student program (GitHub Student Developer Pack, AWS Educate, AWS Academy through the
university) supplies $100–300, the alternative is a single **EC2 t3.micro** running the
existing `docker-compose.yml` — API, Postgres and Redis as three containers on one box, with
a 2 GB swap file because 1 GB of RAM is tight for all three. That is ~$13.50/month
(instance + 30 GB gp3 + public IPv4), closer to the blueprint, and removes the cold start and
the external datastores. It is a strictly worse choice while paying out of pocket.

Opening a second personal AWS account to re-claim the free tier violates AWS's terms — the
free tier is per person, not per account. Not an option.

---

## 4. What the repository needs

None of this exists yet. Listed in dependency order; the allowlist for the slice that builds it.

| # | Path | Change |
|---|---|---|
| 1 | `services/monolith/cmd/server/main.go` | Entrypoint switch: `LAMBDA_TASK_ROOT` set → `ginadapter.NewV2(engine)` + `lambda.Start`; otherwise `engine.Run()` as today. The **only** place that knows about Lambda. |
| 2 | `services/monolith/internal/platform/db/db.go` | Read `DATABASE_URL` (Neon pooled). Pool max 2, `MinConns 0`, short `MaxConnIdleTime`. |
| 3 | `services/monolith/internal/platform/cache/cache.go` | Read `REDIS_URL` (Upstash, TLS). |
| 4 | `services/monolith/internal/platform/health/` | `GET /healthz` — process liveness only, no dependency fan-out; the pipeline smoke-tests it. |
| 5 | `apps/web/.env.production` | `VITE_API_BASE_URL=/api/v1` |
| 6 | `infra/aws/bootstrap.sh` | One-time, idempotent: S3 bucket, OAC, distribution, Lambda, execution role, OIDC provider, deploy role, SSM parameters. |
| 7 | `infra/aws/README.md` | The manual steps bootstrap cannot do (Neon/Upstash signup, GitHub secret). |
| 8 | `.github/workflows/deploy.yml` | §6. |
| 9 | `.github/workflows/destroy.yml` | Manual trigger. Deletes the distribution, bucket and function. |

`docker-compose.yml` is unchanged and remains the local development path. Requiring AWS access
for local development is forbidden by `CLAUDE.md`.

### Configuration

Every value below is an environment variable read through
`internal/platform/config`, sourced from **SSM Parameter Store SecureString** and injected at
deploy time. Never committed, never in `.env.example` with a real value.

| Variable | Source |
|---|---|
| `DATABASE_URL` | Neon pooled connection string |
| `REDIS_URL` | Upstash `rediss://` URL |
| `JWT_PRIVATE_KEY` / `JWT_PUBLIC_KEY` | RS256 keypair, generated once — [`srs-authentication.md`](srs-authentication.md) SEC-02 |
| `DASHSCOPE_API_KEY` | later, when the chatbot domain lands |
| `GIN_MODE` | `release` |

---

## 5. Bootstrap order

One-time, from a workstation with admin credentials. `infra/aws/bootstrap.sh` automates
steps 3–9; steps 1–2 are console work and step 10 is GitHub.

1. **Billing → Budgets: create a zero-spend budget alert.** First, before any resource exists.
   Two budgets are free. This is the only thing standing between a misconfiguration and a bill.
2. Billing → Preferences: enable free-tier and cost-anomaly alerts to email.
3. Region: **`ap-southeast-1`** (Singapore) — latency from Vietnam. CloudFront is global regardless.
4. S3 bucket, block all public access, no versioning.
5. Lambda function, `provided.al2023`, arm64, 512 MB, 10 s timeout, execution role with
   `AWSLambdaBasicExecutionRole` + `ssm:GetParameters` on this project's path prefix only.
6. Lambda Function URL, auth type `AWS_IAM`.
7. CloudFront distribution: S3 origin via OAC; second origin = the function URL, also via OAC;
   `/api/v1/*` cache behavior with caching disabled and `all-viewer-except-host-header` origin
   request policy; default behavior → S3; `redirect-to-https`.
8. CloudFront custom error responses: `403` and `404` → `/index.html` with status `200`.
   Without this, a deep link such as `/dashboard` returns S3's 404 instead of the SPA and
   React Router never runs.
9. GitHub OIDC identity provider + deploy role (§6).
10. GitHub repository variable `AWS_DEPLOY_ROLE_ARN`. **No AWS access keys are ever stored.**

---

## 6. CI/CD — and why it is genuinely free

**Yes, the pipeline costs $0**, provided it stays in GitHub Actions and never touches
CodePipeline or CodeBuild.

| Piece | Cost | Basis |
|---|---|---|
| Actions runners | $0 | Unlimited on public repos; 2,000 min/mo private. A Go build + Vite build is under 3 minutes. |
| GitHub → AWS auth (OIDC) | $0 | IAM roles and OIDC providers are free. |
| `aws s3 sync` | ~$0 | 2.7 MB. |
| `create-invalidation` | $0 | 1,000 paths/month free, permanently. |
| `lambda update-function-code` | $0 | No charge for deployments. |
| Artifact storage | $0 | The zip goes straight to Lambda; nothing is retained. |

The invalidation allowance survives only because Vite content-hashes assets: **invalidate
`/index.html` and nothing else.** Invalidating `/*` on every push burns the monthly quota and
then bills $0.005 per path.

### Workflow shape

```
on: push → main

  test       go vet ./... && go test ./...
             python3 scripts/gen_audit_model.py --check
             python3 scripts/check_i18n_parity.py

  web        needs: test
             npm ci && npm run build
             aws s3 sync dist/assets/ s3://$BUCKET/assets/ --cache-control 'public,max-age=31536000,immutable'
             aws s3 sync dist/ s3://$BUCKET/ --exclude 'assets/*' --cache-control 'no-cache'
             aws cloudfront create-invalidation --paths /index.html

  api        needs: test
             GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap ./cmd/server
             zip -j function.zip bootstrap
             aws lambda update-function-code --zip-file fileb://function.zip
             aws lambda wait function-updated
             curl -fsS https://$DISTRIBUTION/api/v1/healthz
```

The two repo-specific gates in `test` are not optional. `CLAUDE.md` requires
`gen_audit_model.py --check` in CI because the generated Go and TypeScript files must never
be hand-edited, and `check_i18n_parity.py` enforces that no user-facing string exists in one
catalog and not the other.

The `curl` is the deploy gate. A pipeline that reports success without asserting the new code
answers is reporting that an API call was accepted, not that a deploy worked.

### IAM: the two roles

**Deploy role** (assumed by GitHub Actions). Trust policy scoped to
`repo:<owner>/grindstats:ref:refs/heads/main` — an unscoped `token.actions.githubusercontent.com`
trust lets **any** GitHub repository in the world assume it. Permissions: `s3:PutObject`/
`DeleteObject` on the one bucket, `cloudfront:CreateInvalidation` on the one distribution,
`lambda:UpdateFunctionCode`/`GetFunction` on the one function. Nothing else.

**Execution role** (assumed by Lambda). Logs plus `ssm:GetParameters` on
`/grindstats/*` only.

---

## 7. Operational notes

- **Logs.** CloudWatch Logs, 5 GB/month free ingestion. Set a **retention policy of 7 days**
  on the log group — the default is "never expire" and storage is billed. Never log a token,
  only its `jti` (`CLAUDE.md`, audit rules).
- **Audit records are unaffected by deployment.** They are written in `en-US` always,
  regardless of region, request locale, or where the process runs.
- **Rollback** is `aws lambda update-function-code` with the previous commit's artifact, or
  re-running the previous successful workflow. Keep the last few Lambda versions.
- **Teardown.** `destroy.yml` removes the AWS side. Neon and Upstash are deleted from their
  own consoles. Deleting a CloudFront distribution requires disabling it first and waiting.
- **Cost check.** Billing → Bills, monthly. Anything above ~$0.05 means something was created
  that this document does not describe.

---

## 8. Definition of done

Observable, not a matter of taste:

- [ ] A zero-spend budget alert existed **before** the first resource was created.
- [ ] Opening `https://<distribution>.cloudfront.net/` serves the SPA over HTTPS.
- [ ] A deep link (`/dashboard`) loads the SPA rather than an S3 404.
- [ ] `GET /api/v1/healthz` through the distribution returns 200.
- [ ] The Lambda Function URL is **not** reachable directly — `curl` against it returns 403.
- [ ] The S3 bucket is **not** reachable directly.
- [ ] No AWS access key exists in GitHub secrets; the deploy role's trust policy names the
      repository and the `main` ref.
- [ ] No secret value appears in the repository, the workflow file, or the Lambda's plaintext
      environment variables.
- [ ] A push to `main` deploys both halves and the smoke test gates the run.
- [ ] `docker compose up` still runs the whole stack locally with no AWS credentials.
- [ ] Both language catalogs and the generated audit model pass their checks in CI.

---

## 9. Facts to re-verify before trusting this document

| Fact | Where to check | As of 2026-09-07 |
|---|---|---|
| Free tier status of this account | Billing → Free Tier; Billing → Credits | Expired, $0 credits |
| Account signup month | Billing → Bills, oldest month in the dropdown | Predates 15 Jul 2025 |
| CloudFront always-free allowances | AWS free tier page | 1 TB / 10M req / 1k invalidations |
| Lambda always-free allowances | AWS free tier page | 1M req / 400k GB-s |
| Neon, Upstash free tiers | Their pricing pages | 0.5 GB / 256 MB |
| Student credits available | GitHub Student Pack; university AWS Academy status | Not checked |

# GrindStats — Session Handoff

**Written:** 2026-09-03 · **For:** a fresh Cowork session with `C:\PERSONAL PROJECTS\grindstats`
connected and a git remote configured.

This document is deliberately exhaustive. It is written so that a new session can be
fully productive **without** access to the prior conversation, and — if the published
blueprint were ever lost — could reconstruct the design from this file alone. Read
§0 first, then work from the section you need.

---

## 0. Orientation — read this first

**What exists:** a complete, published architecture/design blueprint; an authentication
SRS; repo scaffolding docs. **What does not exist:** a single line of application code.

**Environment facts:**
- User: Aurelius. Git identity **`aureliustran`**. Works on **Windows**.
- Local repo: **`C:\PERSONAL PROJECTS\grindstats`** (created with `git init`).
- **No Mac, no iPhone, no iPad.** Any iOS guidance must assume zero Apple hardware.
- The prior session had file-bridge access to the folder but **no shell access** on the
  user's machine (no `device_bash`), and computer-use was disabled. If your session has
  a shell, you can do far more directly — verify with a `git status` before assuming.

**First actions to take (in order):**
```bash
cd "C:\PERSONAL PROJECTS\grindstats"
git status                      # is anything committed yet?
git log --oneline               # probably empty
git remote -v                   # probably empty — no remote configured yet
git config user.name            # expect: aureliustran
git config user.email           # verify it is set
```
Then: commit the three existing docs, create/attach the remote, push, and create
`docs/blueprint-url.txt` (referenced by `CLAUDE.md`, does not yet exist).

**The one workflow rule the user set explicitly, which was violated once and corrected —
respect it:** *stage new material in markdown documents by default; only edit or
republish the HTML blueprint when the user explicitly says to* (e.g. "update the html",
"add into the html"). Do not "helpfully" sync the blueprint on your own initiative.

---

## 1. Product definition

**GrindStats** — a health/fitness coaching platform for a solo, fresher-level developer
to build. Not a team project; not a startup MVP with funding. Sequencing and
simplification matter more than architectural purity.

**Name history** (older notes may use these): unnamed "Routine, Diet & Calorie Coach" →
**GainsMaxx** → **GrindStats** (current, final). Two earlier candidate names —
*MoggingRealm* and *Lookmaxxing* — were rejected as tied to toxic appearance-obsession
internet culture; the brief was "that energy, but progress-driven."

**Feature pillars:**
1. **Body metrics** — weight, body fat %, muscle mass, circumference measurements, at a
   **user-configurable cadence per metric**.
2. **Training, split into two modalities** — *resistance* (measured for muscle/strength
   stimulus) and *cardio* (measured for energy expenditure, including EPOC "afterburn").
3. **Meal-photo estimation** — Qwen-VL reads photos taken at two compulsory angles and
   returns a structured calorie/macro draft the user edits and confirms.
4. **RAG chatbot** — Qwen text model that proposes and adapts routines using the user's
   own computed data, never inventing numbers.
5. **Routine agreement → live checklist execution** — chatbot proposes a routine, the
   user confirms it, and each scheduled period becomes a checkable occurrence supporting
   ad-hoc task additions mid-session.

**Stack:** Go + Gin microservices behind one API gateway/BFF · React SPA · PostgreSQL ·
Elasticsearch (full-text + `dense_vector` kNN for RAG + operational log) · Redis ·
RabbitMQ (topic exchange) · Qwen via DashScope (text + Qwen-VL vision) · Docker Compose
locally · AWS managed services in prod (**no Terraform/CDK — high-level guidance only**).

---

## 2. Asset inventory — exactly what exists and where

### 2.1 Published blueprint (authoritative spec)

**URL:** `https://claude.ai/code/artifact/f9f2f648-13a6-470c-8ab7-eaad15614682`

A self-contained HTML page: inline CSS with light/dark theming, Google Fonts (Big
Shoulders Display / IBM Plex Sans / IBM Plex Mono), sticky TOC sidebar, mermaid diagrams
rendered natively via `<pre class="mermaid">`, hand-authored inline SVG charts, favicon 🗺️.

**To update it:** call the Artifact tool with `url` set to the above (read it first —
publishing to an artifact this session hasn't read is refused). The prior session's
working copy was `plan.html` in its cloud workspace; a new session must re-read the
published artifact to get current content.

**Section index (22 sections — reference these by number, the doc cross-references itself):**

| § | Title | Contains |
|---|---|---|
| 1 | How to use this plan | Two-layer framing: target architecture vs. build roadmap. "Never introduce two unfamiliar pieces of infrastructure in the same week." |
| 2 | Target architecture | Mermaid flowchart of SPA → gateway → 9 domain services, Postgres/Redis/ES/RabbitMQ/S3/Qwen/POS edges |
| 3 | Service catalog | 10-service table (see §3 below) |
| 4 | Body metrics, training & analytics | What's-tracked table, resistance/cardio split, energy-expenditure formulas + pipeline diagram, measurement-cadence table, 3 SVG charts, weekly-insight sequence diagram |
| 5 | Meal photo estimation | Shooting-angle SVG schematic, required/optional input table, estimation sequence diagram, prompt/output contract, accuracy caveat |
| 6 | Routine agreement & live execution | Template/occurrence/task model, 5-table schema, occurrence lifecycle stateDiagram, agreement + execution sequence diagrams, ad-hoc promotion rule |
| 7 | RAG chatbot & Qwen integration | RAG sequence diagram, app-managed vs BYO-key table, retrieval index design, 6 external source links |
| 8 | Auth, linked accounts & POS subscriptions | Auth/billing sequence diagram, JWT+OAuth2 mechanics, **cookie + Redis blacklist subsection**, POS billing |
| 9 | Elasticsearch | One cluster three jobs; 4 starter indices |
| 10 | Redis | Blacklist/session, rate limiting, hot-read cache, optional pub/sub |
| 11 | RabbitMQ | Topic-exchange diagram, 10-row event catalog |
| 12 | Data model overview | Per-domain table listing (see §5 below) |
| 13 | Quantitative foundations | Governing principle, adaptive TDEE + worked example, 21-day floor, volume landmarks, 8-rule advice engine, LLM payload contract, standing caveats |
| 14 | Project structure | Monorepo tree (see §4 below) |
| 15 | API conventions | REST+JSON, `/api/v1/`, OpenAPI via swaggo, error envelope, pre-bucketed aggregates, two-step writes |
| 16 | Docker & local dev | One root compose file, multi-stage Dockerfiles, `.env.example`, MinIO for local S3 |
| 17 | CI/CD pipeline | GitHub Actions flow, incl. unit-testing `libs/physiology` against worked examples |
| 18 | AWS deployment | Managed-services mapping table (see §11 below) |
| 19 | Security basics | — |
| 20 | Build roadmap | 15 phases, 0–14 (see §10 below) |
| 21 | Learning order | Topic → what to learn → needed-by-phase table |
| 22 | Risks & simplification fallbacks | 12-row "if this gets too heavy → fall back to" table |

### 2.2 Repo contents (as of handoff)

```
C:\PERSONAL PROJECTS\grindstats\
├── CLAUDE.md                       # repo conventions for AI assistants — committed content below
├── README.md                       # human-facing overview
└── docs\
    └── srs-authentication.md       # SRS-AUTH-001, v1.0 — full spec, summarized in §8 below
```

Nothing else. No `.gitignore`, no `docs/blueprint-url.txt` (though `CLAUDE.md` references
it), no `docs/adr/`, no code, no `docker-compose.yml`, no remote.

### 2.3 Staged planning documents — **at risk, not in the repo**

These lived only in the prior session's ephemeral cloud workspace. The HTML blueprint
carries **condensed** versions; the full derivations exist nowhere else. **If they matter,
they must be regenerated — assume they are gone.** What each contained, and specifically
what did *not* survive into the blueprint:

| File | Contents | Not carried into the HTML |
|---|---|---|
| `quantitative-foundations.md` | 15 sections: formulas, derivations, measurement protocols, statistics, advice rule engine, validation plan, 11 external citations | Full Keytel et al. HR equations (both sexes), TRIMP, VO₂max estimation, p-ratio math, the full calibration/validation plan, the per-computation data-requirements table |
| `addendum-measurement-cadence.md` | Cadence-general standard-error formula, days-to-verdict derivation, **required switch from sample-domain to time-domain EWMA (τ ≈ 10.1 days) once cadence is configurable**, `measurement_schedules` schema, LLM payload cadence block, corrected intake-completeness gating (soft 70% floor + >15pp-change hard gate, replacing an earlier ≥90% hard gate) | The EWMA time-domain requirement and the intake-completeness gate details are only summarized |
| `addendum-routine-execution.md` | Full routine schema, occurrence lifecycle, agreement/execution flows, ad-hoc promotion, **late-logging grace windows**, event & API additions | The late-logging grace-window rules (24h edit window on `completed`; beyond that, an explicit "edit past workout" action, with metrics writes timestamped to the *occurrence date* not the edit time, so trend math isn't corrupted) |
| `addendum-session-redis.md` | Cookie + Redis blacklist auth design | Fully merged — nothing lost |
| `notes-training-types.md` | Resistance/cardio split rationale | Superseded by §4; nothing lost |

**Recommendation:** early in the new session, recreate the still-valuable ones (especially
the quantitative derivations and the late-logging rules) as files under `docs/` so they
are version-controlled rather than ephemeral.

---

## 3. Service catalog (blueprint §3)

Ten services. **In the roadmap these start as packages inside one Gin binary** and are
extracted one at a time; this table is their end state.

| Service | Responsibility | Store | Notes |
|---|---|---|---|
| **gateway** | Routing, JWT validation, rate limiting, fan-out | — | Thin; no business logic |
| **auth-service** | Signup/login, JWT issue & refresh, OAuth2 social login | Postgres, Redis | Redis holds blacklist/session state |
| **user-service** | Profile, preferences, units (metric/imperial), goals | Postgres | Source of truth for "who is this user" |
| **routine-service** | Habits, routines, schedule items, reminders, **live occurrence checklists** | Postgres | Publishes `routine.*` |
| **nutrition-service** | Food log, calorie intake, macros, **meal-photo estimation via Qwen-VL** | Postgres, S3 | Publishes `calorie.*` |
| **metrics-service** | Body composition; resistance & cardio training logs; **BMR/MET/EPOC energy calcs**; trend, cadence & aggregation endpoints | Postgres | Publishes `body_metric.*`, `training.*` |
| **diet-service** | Diet plans, meal templates, macro targets | Postgres | Reads nutrition aggregates via API, not shared tables |
| **chatbot-service** | RAG orchestration, Qwen calls, conversation history, **routine proposals**, weekly insight job | Elasticsearch, Redis | — |
| **subscription-service** | Plans/entitlements, POS webhooks, linked Qwen & social accounts | Postgres | — |
| **notification-service** | Push/email reminders, digests | Postgres (light) | Pure event consumer; no public API at first |

**Rule:** each service owns its tables. Even in one shared Postgres instance (the
recommended solo shortcut — one schema per service: `auth.*`, `routine.*`, `metrics.*`,
`nutrition.*`, …), **no service reads another's tables directly.**

---

## 4. Repo structure (blueprint §14)

```
grindstats/
├── apps/
│   └── web/                     # React SPA
│       ├── src/
│       │   ├── features/
│       │   │   ├── routines/    # checklist UI, occurrence view, ad-hoc add
│       │   │   ├── nutrition/   # food log + camera capture, angle guide, estimate review
│       │   │   └── analytics/   # dashboard: trend & volume charts
│       │   ├── api/             # generated/typed API client
│       │   └── app/             # routing, layout, auth context
│       └── package.json
├── services/
│   ├── gateway/                 # Gin: routing, JWT check, rate limit
│   ├── auth-service/
│   ├── user-service/
│   ├── routine-service/
│   ├── nutrition-service/
│   ├── metrics-service/
│   ├── diet-service/
│   ├── chatbot-service/
│   ├── subscription-service/
│   └── notification-service/
├── libs/                        # shared Go modules
│   ├── authmw/                  # JWT verification middleware
│   ├── eventbus/                # RabbitMQ publish/consume helpers
│   ├── qwenclient/              # shared Qwen text + vision client
│   ├── physiology/              # BMR/MET/EPOC constants & formulas — ONE source of truth
│   └── httpkit/                 # error handling, request logging
├── infra/
│   ├── docker/                  # Dockerfiles + docker-compose.yml
│   └── ci/                      # GitHub Actions workflows
├── docs/
│   └── adr/                     # architecture decision records
└── README.md
```

**Until Phase 10, most of `services/` is one Go module — `services/monolith/` — with
internal packages named after the future services.** The tree above is where each
package's contents move *to*, not where you start.

---

## 5. Data model (blueprint §6, §12)

### 5.1 Routine & execution — the fully specified part

```
routines
  id, user_id, name, status ENUM(draft, active, paused, completed),
  source ENUM(manual, chatbot_suggested), goal_summary TEXT, created_at

routine_periods                          -- the template
  id, routine_id, label, kind ENUM(resistance, cardio, habit, rest),
  recurrence_rule TEXT,                  -- RFC 5545 RRULE, e.g. "every Mon/Thu"
  planned_time TIME NULL, sort_order INT

task_templates                           -- planned items within a period
  id, period_id, label, sort_order,
  exercise_ref TEXT NULL,
  target JSONB NULL                      -- {"sets":3,"reps":8,"load_note":"RPE 8"}
                                         -- {"duration_min":30,"zone":"z2"}

period_occurrences                       -- one concrete instance, one date
  id, period_id, occurrence_date,
  status ENUM(upcoming, active, completed, skipped),
  started_at, completed_at

occurrence_tasks                         -- what the user sees and checks
  id, occurrence_id,
  task_template_id NULL,                 -- NULL ⇒ ad-hoc, added during execution
  label, sort_order, target JSONB NULL,
  status ENUM(unchecked, checked, skipped),
  is_adhoc BOOL DEFAULT false,
  checked_at NULL,
  training_set_ids UUID[] NULL,          -- links to metrics-service rows
  added_to_template BOOL DEFAULT false   -- true if promoted into the standing plan
```

**Why `occurrence_tasks.target` is a copy, not a reference:** if the template changes next
week, past occurrences must still show what was planned *at the time*. Cloning at
generation time makes history immutable without an audit table.

**Occurrence lifecycle:** `[*] → upcoming` (generated ahead by a nightly job) →
`active` (planned time reached, or user opens early) → `completed` (user taps Finish, or
day rolls over) **or** `skipped` (explicit skip, or window passes untouched from
`upcoming`). Check/uncheck/add are permitted only in `active`, plus a 24h grace window
into `completed` for late logging. `upcoming` is a read-only preview.

**Behavior at check time:** for a `resistance`/`cardio` task, checking opens the structured
log pre-filled from `target`; confirming writes to metrics-service **and** sets
`status=checked` with returned row ids in `training_set_ids`. For a `habit` task it's just
a boolean — no metrics-service call.

### 5.2 Other domains

| Domain | Tables | Key relationships |
|---|---|---|
| Identity | `users`, `oauth_identities` | `oauth_identities.user_id → users.id` |
| Nutrition | `food_items`, `calorie_logs`, `meal_photos` | `calorie_logs.food_item_id → food_items.id`; `calorie_logs.source ∈ {manual, photo, barcode}`; `meal_photos.calorie_log_id → calorie_logs.id` |
| Body & training | `body_metrics`, `training_sessions`, `training_sets`, `cardio_details`, `measurement_schedules` | `training_sessions.type ∈ {resistance, cardio}`; `training_sets.session_id →` (resistance only); `cardio_details.session_id →` (1:1, cardio only); `measurement_schedules` holds per-user cadence |
| Diet | `diet_plans`, `meal_templates` | `diet_plans.user_id → users.id` (cross-service, via API not FK) |
| Subscription | `entitlements`, `linked_accounts`, `billing_events` | `linked_accounts.provider ∈ {google, apple, qwen}` |
| Chat | Elasticsearch `events_log` (not relational) | referenced by `user_id` only |

`cardio_details` holds: activity, duration, avg/max HR, intensity zone, `kcal_during`,
`kcal_epoc_est`.

---

## 6. Event catalog (blueprint §11)

RabbitMQ **topic exchange**; three starter queues: `notification.queue`,
`analytics.queue`, `chatbot-context.queue`.

| Routing key | Publisher | Consumers | Payload |
|---|---|---|---|
| `routine.occurrence_started` | routine-service | notification | user id, occurrence id, period label |
| `routine.task_checked` / `.unchecked` | routine-service | analytics, chatbot | user id, task id, kind, linked `training_set_ids` |
| `routine.task_added` | routine-service | analytics | user id, task id, `is_adhoc=true` |
| `routine.occurrence_completed` | routine-service | notification, chatbot, analytics | user id, checked/unchecked/adhoc counts, duration |
| `calorie.logged` | nutrition-service | analytics, chatbot | user id, food items, kcal, source (manual/photo), timestamp |
| `body_metric.logged` | metrics-service | chatbot, analytics | user id, metric type, value, timestamp |
| `training.logged` | metrics-service | chatbot, analytics | user id, session type (resistance/cardio), summary (tonnage **or** duration+kcal), timestamp |
| `subscription.updated` | subscription-service | auth, notification | user id, new tier, effective date |
| `chat.completed` | chatbot-service | analytics | user id, conversation id, token usage |
| `insight.generated` | chatbot-service | notification | user id, insight id, summary |
| `notification.reminder_due` | notification-service (internal, delayed) | notification | user id, reminder text, channel |

Note: `routine.task_checked` on a resistance/cardio task fires **after** metrics-service
returns `training_set_ids`, so consumers never join across services.
`routine.occurrence_completed` carries the checked/unchecked/adhoc triple because that's
exactly what the weekly-insight job and the adherence percentage both need.

---

## 7. Quantitative layer (blueprint §13) — exact formulas and constants

**Governing principle: "Services compute. The LLM narrates."** Every number reaching a
user is produced by deterministic Go in metrics-service, stored, and handed to Qwen
pre-computed. The model explains and prioritizes; it never calculates.

### 7.1 Energy expenditure

| Component | Formula |
|---|---|
| BMR (primary) | Katch-McArdle: `370 + 21.6 × lean_mass_kg`, where `lean_mass_kg = weight_kg × (1 − body_fat_%/100)` |
| BMR (fallback, no body-fat reading in 30 days) | Mifflin-St Jeor: `10×weight + 6.25×height_cm − 5×age + 5` (men) / `− 161` (women) |
| Resistance session | `(MET − 1) × 1.05 × weight_kg × duration_hr`, MET ≈ 3.5–6 |
| Cardio session | MET-based; upgrade to Keytel et al. (2005) HR regression once HR data exists (valid ≈ 90–150 bpm steady state) |
| EPOC | `k × session_kcal`, `k` ≈ 0.07 (steady cardio) to 0.13 (intervals), **capped at 0.15**; typically 30–80 kcal absolute |

### 7.2 Adaptive TDEE (the self-correcting estimator)

```
TDEE_est = mean_daily_intake − (Δtrend_weight_kg × 7,700) / N_days

Worked example — 28-day window:
  mean logged intake       = 2,400 kcal/day
  weight trend (smoothed)  = 82.4 kg → 81.6 kg  (Δ = −0.8 kg)
  energy from stores       = 0.8 × 7,700 = 6,160 kcal
  per day                  = 6,160 / 28  =   220 kcal/day
  TDEE_est                 = 2,400 + 220 = 2,620 kcal/day
```

**Key property — it cancels systematic logging bias.** Under-log by 15% → estimate is
15% low → prescribed target is 15% low → the errors cancel and the user still eats at the
intended deficit. This is *why* photo estimation's ±20–30% error doesn't undermine the
system. What breaks it: a **change in logging habit mid-window** — a `calorie_logs.source`
mix shift of **>25 percentage points** invalidates calibration and restarts the window.

**Gates before trusting it over the formula prior:** window ≥ 21 days · weight trend
statistically significant · no diet-composition step-change in the last 14 days.

### 7.3 Statistics

- Slope standard error scales as **`σ / (T^1.5 × √f)`** — window length `T` at the 3/2
  power, frequency `f` only as a square root. **Waiting longer buys far more certainty
  than measuring more often.**
- **Hard rule: no calorie or training prescription change on fewer than 21 days of data,
  at any cadence.** At 7 days, measurement error exceeds double the effect size.
- **Smoothing: time-domain EWMA (τ ≈ 10.1 days), not sample-domain** — mandatory once
  cadence is user-configurable, or irregular logging silently distorts the trend.
- Days-to-confident-trend (0.5% bodyweight/week at 2σ): 7×/wk → ~21d · 5× → ~24d (+12%) ·
  3× → ~28d (+33%) · 1× → ~40d (+92%).

### 7.4 Resistance volume landmarks

Measured in **weekly hard sets per muscle group** (not tonnage — tonnage is confounded by
exercise choice).

| Landmark | Sets·muscle⁻¹·week⁻¹ | Meaning |
|---|---|---|
| MV — maintenance | ~6 | preserves, grows little |
| MEV — minimum effective | ~8–10 | growth floor for most trained lifters |
| MAV — maximum adaptive | ~12–20 | the productive working range |
| MRV — maximum recoverable | ~20+ | above this, recovery fails |

Progress is read from **three series together**: FFMI trend, e1RM slope per lift
(Epley/Brzycki, valid ≤10 reps), and weekly hard sets vs. landmarks.

**Weekly sets are computed from *checked* occurrence_tasks with linked training logs** —
so a skipped planned set is excluded, while an unplanned logged set still counts.

### 7.5 Advice rule engine (deterministic; the LLM only selects/sequences/explains)

| Condition | Output |
|---|---|
| Loss rate > 1.0% bodyweight/week, sustained ≥ 2 weeks | Reduce deficit 15–20% (lean-mass retention risk) |
| Plateau detected, logging/weigh-in gates passed | Reduce intake 5–10%, or add ~1,500 steps/day |
| Plateau detected, gates **not** passed | Adherence conversation — no target change |
| Weekly hard sets below MEV, FFMI flat | Add 2–4 sets/week to that muscle |
| Weekly hard sets above MRV, e1RM slope negative | Deload — cut volume ~50% for one week |
| Weekly training load jumps >30% over trailing 4-week mean | Flag as information, suggest gentler ramp (**not** an injury prediction) |
| Adaptive TDEE diverges >15% from formula prior | Surface as logging-accuracy discussion, **not** a metabolism claim |
| Logging source mix shifts >25pp between windows | Restart estimator window; tell the user why |

### 7.6 LLM payload contract

```json
{
  "energy": { "tdee_est_kcal": 2620, "tdee_ci95": [2396, 2844],
              "prescribed_intake_kcal": 1960, "method": "adaptive" },
  "composition": { "weight_trend_kg_per_week": -0.58, "significant": true,
                   "ffmi_normalised": 19.4 },
  "resistance": { "weekly_sets": { "chest": 14, "hamstrings": 6 },
                  "landmarks": { "mev": 8, "mav": [12, 20] } },
  "triggered_rules": [5, 8],
  "low_confidence_fields": []
}
```

System-prompt constraints: use only numbers present in the payload; never restate a
low-confidence value without its caveat; address `triggered_rules` in order; invent
nothing beyond them; cite the number behind every recommendation.

**The hallucination guard (implement this — it's a checked invariant, not a hope):** a
post-generation **Go** check regex-extracts every numeral from the LLM's output and
asserts each appears in the payload. An orphan number → regenerate once → then fall back
to a template-rendered response.

### 7.7 Standing caveats — keep attached in code comments *and* user-facing copy

- Population-level sports-science estimates, **not** measurement: BMR ±10%, MET-based
  expenditure ±15–25%, consumer body-fat readings ±3–4 percentage points.
- The adaptive estimator needs 3+ weeks of consistent data to beat the formula prior, and
  breaks silently when logging habits change — hence the source-mix monitor.
- **Not a clinical tool.** Never prescribe below BMR; absolute floors ≈ 1,500 kcal (men) /
  1,200 kcal (women).
- **ACWR (acute:chronic workload ratio) was evaluated and deliberately rejected** as a
  decision rule, due to documented methodological criticism. Do not reintroduce it. The
  >30% load-jump rule above is its deliberately weaker, information-only replacement.

---

## 8. Authentication (blueprint §8 + `docs/srs-authentication.md`)

### 8.1 The design

**Problem:** a signature-only JWT can't be revoked before expiry, gives no session
visibility, and no "log out everywhere."

**Chosen solution — the blacklist strategy.** Every token carries a unique `jti`.
Revocation = a Redis write. The **gateway** is the only component that checks Redis;
downstream services verify signature only, preserving statelessness internally.

**Redis keys:**

| Key | Value / TTL | Purpose |
|---|---|---|
| `blacklist:{jti}` | `"1"`, TTL = token's remaining life | Revoke one token: logout, refresh rotation, single compromised token. Self-expiring — no cleanup job. |
| `user_blacklist_epoch:{user_id}` | unix timestamp, TTL = max refresh life | **O(1) revoke-all**: any token with `iat` < epoch fails. Logout-everywhere, credential change, refresh-reuse compromise. |
| `user_sessions:{user_id}` *(optional)* | set of `{jti, device_label, created_at}` | "Active sessions" screen. Not on the hot path. |

**Token model:**

| | Access | Refresh |
|---|---|---|
| Format | JWT RS256 | JWT RS256 |
| Lifetime | 10–15 min | 30 days absolute, 14 days idle |
| Claims | `sub`, `role`, `tier`, `jti`, `iat`, `exp` | `sub`, `jti`, `iat`, `exp`, `typ:"refresh"` |
| Transport | httpOnly, Secure, SameSite=Lax cookie | same, path-scoped to `/api/v1/auth` |

Because tokens ride in cookies, **state-changing requests require a double-submit CSRF
token** (`X-CSRF-Token` header, checked against the session-bound value).

**Gateway check order** (401 `AUTH_INVALID_TOKEN` on failure): signature → expiry →
`blacklist:{jti}` present → `iat` < `user_blacklist_epoch`.

**Refresh rotation:** each refresh mints a new pair and immediately blacklists the old
refresh `jti`. **A blacklisted refresh token being presented = replay of a stolen token →
set the user's epoch (kill all sessions), log a WARN security event, return 401.**

### 8.2 Roles — exactly two

| Role | Scope |
|---|---|
| **User** | Default for every account. Own data only; the effective user id always comes from the token, never from a client-supplied parameter. |
| **SystemAdmin** | Accounts and sessions only: suspend/unsuspend, force-logout, view session lists and security events, grant/revoke roles. **Explicitly cannot read or modify users' health data** (FR-47). Never self-assignable through any registration path (FR-04); grants are API/ops only, not exposed in the SPA (FR-46). All admin actions are append-only audit-logged (FR-45). |

### 8.3 SRS-AUTH-001 structure

`docs/srs-authentication.md` contains: §1 introduction/scope/definitions/roles · §2
overall description (context, token model, assumptions) · §3 functional requirements with
**stable IDs — do not renumber**: FR-01..08 registration, FR-10..14 login/issuance,
FR-20..24 request auth & authorization, FR-30..34 refresh/rotation/logout, FR-40..41
session visibility, FR-42..47 SystemAdmin, plus a full endpoint table · §4 NFR-01..07
(latency budgets, Redis-down policy: **fail closed on state-changing, fail open
signature-only on GETs**, blacklist hygiene, rate limits, testability with miniredis,
observability) · §5 SEC-01..07 (argon2id, key rotation via `kid` + two-key window, cookie
flags, single-use hashed email tokens, PKCE + exact-match redirect allowlist) · §6 eight
representative acceptance criteria · §7 traceability table back to blueprint sections.

**Known gap:** the SRS assumes browser cookie transport. Native mobile clients don't share
the cookie jar — see §12.2 below.

---

## 9. API conventions (blueprint §15) and known endpoints

**Conventions:** REST + JSON everywhere · versioned `/api/v1/...` from day one ·
OpenAPI per service generated from Gin route comments (swaggo) → free typed client for
React · **error envelope `{ "error": { "code": "...", "message": "..." } }` everywhere** ·
**aggregation endpoints return pre-bucketed series** (e.g.
`GET /metrics/trend?metric=body_fat&interval=week` → `[{week, value}]`, not raw rows) ·
**two-step writes for anything LLM-generated**.

**Known endpoints:**

```
# Auth (from SRS-AUTH-001 §3.7, all under /api/v1/auth)
POST /register · POST /login · GET /oauth/google[/callback] · POST /verify-email
POST /password-reset/request · POST /password-reset/confirm
POST /refresh · POST /logout · POST /logout-all
GET  /sessions · DELETE /sessions/{id}
POST /admin/users/{id}/suspend|/unsuspend|/force-logout   (SystemAdmin)
GET  /admin/users/{id}/sessions|/security-events          (SystemAdmin)
PUT  /admin/users/{id}/role                               (SystemAdmin)

# Two-step LLM writes — NEVER collapse these into one call
POST /meals/estimate     → POST /meals/confirm
POST /routines/propose   → POST /routines/confirm

# Routine execution
GET   /occurrences/today
PATCH /occurrences/{id}/tasks/{taskId}     # check / uncheck / edit
POST  /occurrences/{id}/tasks              # add ad-hoc task
POST  /occurrences/{id}/complete
POST  /periods/{id}/task_templates         # promote ad-hoc into standing plan
```

---

## 10. Build roadmap (blueprint §20) — 15 phases

Assumes ~15–20 hrs/week. **Order matters more than the week estimates.**

| Phase | Title | Est. | Content |
|---|---|---|---|
| 0 | Foundations | 2–3w | Go+Gin, React, Docker, git basics. One throwaway hello-world API + page. |
| 1 | Modular monolith MVP | 6–8w | One Gin binary, one Postgres. Internal packages: auth, user, routine (incl. occurrences + checklists, **manually created — no chatbot yet**), nutrition, diet, body-metric/training CRUD. **Flat BMR/PAL formula** as the first "calories out". React SPA for login + logging. No Redis, no queue, no chatbot, no photos. |
| 2 | Real auth | 2w | Proper JWT access+refresh, Redis, Google OAuth2. **(Implement SRS-AUTH-001 here.)** |
| 3 | First async hop | 2–3w | Introduce RabbitMQ. Extract **notification-service** first — lowest-risk service to get wrong, proves publish/consume end-to-end. |
| 4 | Search | 2w | Elasticsearch; index food items + routines full-text. No vectors yet. |
| 5 | Chatbot v1 + routine agreement | 4w | Qwen text API with recent data stuffed into the prompt (no vectors). Then the routine propose/confirm flow — the chatbot's first write action. |
| 6 | Meal photo estimation | 3–4w | Same Qwen pattern, vision model. Camera UI with two required angles, S3 presigned upload, structured-JSON estimate, estimate/confirm review. |
| 7 | Real RAG | 2–3w | `dense_vector` + embeddings pipeline; swap prompt-stuffing for kNN retrieval. |
| 8 | Analytics dashboard, adaptive TDEE & AI insights | 4–5w | Charting UI (composition trend, calories in/out, resistance volume). Adaptive TDEE, volume landmarks, rule engine, configurable cadence. "Calories out" stops being a placeholder. Then the weekly insight job. |
| 9 | Subscriptions & linked accounts | 3–4w | Stripe checkout + webhooks, entitlement gating on chatbot + photo estimation, Qwen key linking. |
| 10 | Finish the split | 4–5w | Extract user, routine, nutrition, metrics, diet behind the gateway, using the Phase-3 pattern. |
| 11 | Full containerization | 1w | Dockerfile per service, one compose file, README that gets a stranger running in one command. |
| 12 | CI/CD | 1–2w | GitHub Actions: lint, test, build, scan, push to ECR, auto-deploy staging. |
| 13 | AWS launch | 2–3w | Stand up §18 one piece at a time: RDS → ECS+ALB → S3 → the rest. Never all in one sitting. |
| 14 | Harden | 2–3w | Security review vs §19, load-test both Qwen paths, write the docs. |

---

## 11. AWS mapping (blueprint §18) — managed services only, no IaC

S3 + CloudFront (SPA, meal photos) · **ECS Fargate** (chosen over EKS) · RDS (Postgres) ·
ElastiCache (Redis, same VPC as ECS for latency) · OpenSearch Service · Amazon MQ ·
Secrets Manager · Route 53 + ACM · ECR · CloudWatch · WAF.

---

## 12. Open threads — with the specifics needed to act

### 12.1 Landing page (interrupted mid-request — highest priority)

The user asked for **a prompt** (not the page itself) to build the GrindStats app landing
page; HTML/CSS/JS/TailwindCSS allowed. They attached a One Punch Man training-meme image
("100 PUSH-UPS / 100 SIT-UPS / 100 SQUATS / 10KM RUNNING — EVERY SINGLE DAY (and never
use air-conditioner)") over stark monochrome Saitama line art, and asked for that
"spirit."

**Constraint to honor:** Saitama is a copyrighted character and that is a specific
copyrighted artwork — do not reproduce the character or recreate the image. Channel the
*spirit* instead: stark monochrome/high-contrast, sparse heavy sans typography, the
absurd-simplicity-of-relentless-daily-repetition motif, a hand-drawn/inked texture
feel, the deadpan parenthetical aside as a voice device. An original silhouette or a
purely typographic treatment carries all of it without touching the IP. The
"every single day" cadence maps naturally onto GrindStats' actual daily-checklist feature.

### 12.2 Mobile client strategy (offered, user did not yet accept — do not write unprompted)

Content agreed in conversation, ready to be written up:
- **PWA first** — manifest + service worker on the existing React SPA; installable on both
  iOS/iPadOS (Safari "Add to Home Screen") and Android; no Apple hardware, account, or
  review needed. Browser camera APIs (`getUserMedia` / `<input capture>`) cover meal-photo
  capture. Limits: iOS PWA push only since 16.4 and less reliable; no HealthKit.
- **React Native + Expo later** — EAS Build compiles iOS binaries on Expo's cloud macOS
  machines and EAS Submit uploads to App Store Connect, all driven from Windows.
  Unavoidable: **Apple Developer Program $99/yr** (an account, not hardware). Testing
  without an iPhone: Appetize.io (free tier), BrowserStack/LambdaTest real devices, or a
  borrowed device with TestFlight. Android emulator runs fine on the PC.
- **Architecture consequence for auth:** native clients don't share the browser cookie
  jar. They'd hold JWTs in Keychain / Android Keystore and send `Authorization: Bearer`.
  The gateway needs **dual transport** (cookie *or* header); CSRF tokens are unnecessary on
  the header path since nothing is auto-sent. **This belongs in SRS-AUTH-001 as a
  revision** when implemented.

### 12.3 Other queued work

- **Remaining SRS documents** — only auth exists. Warranted: metrics/analytics,
  nutrition + meal photo, routines/execution, chatbot/RAG.
- **`docs/blueprint-url.txt`** — referenced by `CLAUDE.md`, doesn't exist. Create it with
  the artifact URL.
- **Git remote** — none configured; nothing pushed.
- **`.gitignore`** — none. Needed before any code lands (Go build artifacts, `node_modules`,
  `.env`).
- **Phase 0/1 scaffolding** — not started.

---

## 13. Conventions — the non-negotiables

Reproduced from `CLAUDE.md`; these are the rules an assistant is most likely to break by
accident.

1. **Two-step writes for anything LLM-estimated or LLM-proposed.** Never one endpoint that
   generates *and* persists.
2. **"Services compute, the LLM narrates."** No LLM arithmetic, ever. Plus the numeral
   validation guard (§7.6).
3. **Ad-hoc routine edits never silently mutate the template.** Promotion is a distinct,
   user-initiated action (offered after 3 repeats, never automatic).
4. **Unchecking a routine task never auto-deletes a logged training set.** Prompt once.
5. **Formulas live in one place** — `libs/physiology/`, imported everywhere, never
   reimplemented per service.
6. **Consistent error envelope** across all services.
7. **Versioned API paths** from day one.
8. **Aggregation endpoints return pre-bucketed series.**
9. **Check the roadmap phase before building** — don't jump ahead to later-phase
   infrastructure (e.g. don't split services before Phase 10).
10. **Prefer editing `services/monolith/` internal packages** over creating a standalone
    service until the roadmap says to extract one.
11. **Cite the source for any new formula or cadence logic**, and keep the
    population-level/not-clinical caveat attached in both code comments and user copy.
12. **Local dev must never require AWS access.**

### Fallbacks if something proves too heavy (blueprint §22)

Occurrence state machine → plain boolean checklist, no ad-hoc/promotion · BMR/MET/EPOC
math → flat TDEE activity multiplier · configurable cadence → daily-only weigh-ins ·
two photo angles → top-down only, labeled "rough estimate, no scale reference" · RabbitMQ
→ Amazon MQ or Redis Streams · BYO Qwen keys → app-managed key only · ten services → stop
at 5–6 · ECS → Elastic Beanstalk / App Runner · ES vectors → keep prompt-stuffing longer.

---

## 14. Suggested first session plan

1. Verify repo state (commands in §0), commit the three existing docs, add `.gitignore`,
   create the remote, push.
2. Create `docs/blueprint-url.txt` with the artifact URL.
3. Copy this handoff into `docs/` if it isn't already there, and consider regenerating the
   at-risk staged docs (§2.3) into `docs/` as well.
4. Ask the user which they want next: the landing-page prompt (§12.1), the mobile
   addendum (§12.2), another SRS, or Phase 0/1 scaffolding.

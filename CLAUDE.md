# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

MPlatform is a single-binary Kubernetes console for multi-cluster operations and LLM
inference serving (vLLM/SGLang): a Go (gin + GORM) backend, a React/Vite frontend in
`ui/`, and the built frontend embedded from `static/`. Product context is in
`README.md` and `PRODUCT.md`, visual rules in `DESIGN.md`, domain terms in `GLOSSARY.md`.

## Commands

- Dev: `make dev` (backend on :8080 plus Vite on :5173, which proxies `/api/` and `/ws/` to :8080).
- Build: `make build` (= `make frontend` + `make backend`, output `./mplatform`).
- Tests: `make test` (Go + Vitest).
  - One Go test: `go test ./pkg/resources -run TestGetPodMetrics`
  - One UI test file: `pnpm --dir ui exec vitest run src/lib/k8s.test.ts` (add `-t "<name>"` for one case)
- Type-check the UI: `pnpm --dir ui exec tsc --noEmit -p tsconfig.app.json`
  (`tsconfig.node.json` for `ui/vite.config.ts`). Don't use the `type-check` script:
  the root tsconfig has no source files, so it checks nothing.
- Git pre-commit hook: run `scripts/pre-commit-gate.sh install` once per clone (git
  hooks aren't versioned). On every commit, anyone's, it formats the staged files and
  restages them, then lints only the staged files (gofmt/golangci-lint, prettier/oxlint,
  ruff, shfmt/shellcheck; settings in `scripts/pre-commit-gate.conf`) and blocks the
  commit on failure. It does not run `make lint` (ESLint, go vet), so run that yourself
  before committing. Fix the reported problems instead of using `--no-verify`.
- Lint: `make lint` runs `go vet`, golangci-lint (`.golangci.yml`, binary fetched into
  `bin/`) and ESLint in `ui/` and `e2e/`. One part at a time: `bin/golangci-lint run`,
  `pnpm --dir ui run lint`, `pnpm --dir e2e run lint`. The e2e part needs
  `make e2e-install` once.
  - `ui/` uses the typescript-eslint and react-hooks recommended rules (React Compiler
    rules, though the compiler isn't enabled); the `incompatible-library` warnings on
    TanStack Table are expected. To reset or sync state when props change, use
    `useDepsChanged` (`ui/src/hooks/use-deps-changed.ts`) during render instead of
    setState in an effect.
  - `e2e/` uses the typescript-eslint and Playwright recommended rules; its
    warnings are known and don't fail the run.
- E2E (Playwright against a local kind cluster, needs Docker + kind): `make e2e-test`
  the first time, then `make e2e-run SPEC=specs/user-access.spec.ts`.
  `external-auth.spec.ts` is skipped unless `make e2e-setup-ldap e2e-setup-dex` ran and
  `MPLATFORM_E2E_LDAP_URL` / `MPLATFORM_E2E_OAUTH_ISSUER` are set. Model serving uses its
  own multi-node cluster: `make e2e-model-serving-run`, removed by `make e2e-model-serving-down`.

The backend on :8080 serves the frontend embedded at build time, so after a UI change
run `make frontend` and rebuild/restart the backend before checking it there; only
Vite on :5173 updates live.

## Architecture

**One binary, two roles.** `main.go` runs the `cluster-agent` or `alert-forwarder`
subcommand (run inside managed clusters from the same image), otherwise the web
server. `app.go` `initializeApp` sets up in order: env (`pkg/common/common.go`) →
GORM DB (`pkg/model`; SQLite by default, MySQL/Postgres via `DB_TYPE`/`DB_DSN`) →
RBAC and templates → the optional YAML config file (`MPLATFORM_CONFIG_FILE`,
`internal/`, watched for changes) → `cluster.ClusterManager` → background
`scheduler` and `provision` loops.

**Routes** (`routes.go`, all under the base path from `MPLATFORM_BASE`):
- public: `/healthz`, `/metrics`, `/api/auth/*`, bootstrap, cluster-agent registration;
- `/api/users/*` for the current user, `/api/v1/admin/*` behind `RequireAdmin`;
- cluster APIs, registered twice by `registerClusterProtectedRoutes`: under
  `/api/v1/...` and under `/api/v1/_clusters/:cluster/...`.

**Cluster selection.** `middleware.ClusterMiddleware` takes the cluster from the
`:cluster` path param, then the `x-cluster-name` header, then the query param, and
stores the `*cluster.ClientSet` (K8s, Prometheus, Alertmanager clients) in the gin
context under `"cluster"`. `ClusterManager` builds ClientSets from DB cluster rows
(kubeconfig, in-cluster, or a cluster-agent tunnel via `pkg/clusteragent`).

**RBAC gotcha.** `middleware.RBACMiddleware()` is attached right before
`resources.RegisterRoutes`, so it guards only the generic resource routes, deriving
verb/namespace/resource from the URL. Everything registered above it (logs,
terminals, metrics, proxy, helm, AI, and the modelserving/monitoring/logging
handlers) checks permissions itself. AI tool calls are authorized in
`pkg/ai/tool_authorization.go`.

**Resource handlers.** `pkg/resources/handler.go` maps each resource name to a
handler: mostly `NewGenericResourceHandler[T, TList]`, `newVersionedResourceHandler`
trying API versions in order, and dedicated handlers for pods, nodes, workloads,
events and Helm releases. Paths are `/<resource>/:namespace/:name`; `_all` means all
namespaces or cluster scope. Unknown names fall through to the CRD handler
(`/:crd/...`). A new or renamed resource must stay in sync across
`pkg/common/resource.go`, `pkg/resources/handler.go`, `ui/src/lib/resource-catalog.ts`
and its UI routes/pages.

**Feature modules** expose a `Handler` with `RegisterRoutes`/`RegisterAdminRoutes`:
`pkg/modelserving` (inference services, LeaderWorkerSet and PD-disaggregated
deployments, Higress gateway, GPU Operator/HAMi installs), `pkg/monitoring`,
`pkg/logging`, `pkg/helm`, and `pkg/provision` (kubeadm cluster creation over SSH).

**Frontend.** `ui/` builds into `../static`, embedded by `static.go`; unknown non-API
paths fall back to `index.html` with the base path injected. API calls live in
`ui/src/lib/api/*` on top of `apiClient` (`lib/api-client.ts`: token refresh,
sub-path); the selected cluster is sent as `x-cluster-name` or a `/_clusters/<name>`
prefix (`lib/current-cluster.ts`). `ui/src/components/ui` holds shadcn-managed
primitives. User-visible text goes through i18n; keep `ui/src/i18n/locales/en.json`
and `zh.json` in sync. The console is desktop-only.

**Deployment.** Helm chart in `charts/mplatform`, add-on charts in `charts/addons`.
A new runtime env var belongs in both `pkg/common/common.go` and the chart's
values/templates.

## Project rules

- Persist sensitive values with `model.SecretString`.
- Settings applied from the config file must stay read-only in the UI.
- Don't hand-edit generated output: `static/`, `mplatform`, `bin/`. Touch `go.sum` and
  the `pnpm-lock.yaml` files only for dependency changes.

## Code graph

`graphify-out/graph.json` is a knowledge graph of the repo (a hook in
`.claude/settings.json` asks for it before Grep/Read). Start code questions with
`graphify query "<question>"`, `graphify explain "<concept>"` or
`graphify path "<A>" "<B>"`, and run `graphify update .` after code changes. The
committed `graph.json` is rewritten whole on every build, so its large diffs are expected.

## Agent skills

### Issue tracker

Issues live in GitHub Issues for `qzytwway/mplatform`, managed with the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five default triage labels, each named after its role (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

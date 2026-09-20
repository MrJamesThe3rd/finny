# finny — Knowledge Base

Durable reference for what finny is, what it does today, and the domain facts that
shape it. Future work lives in [roadmap.md](roadmap.md).

Last verified: 2026-09-20 — tenancy, audit and the frontend org switcher are in
`main` and were checked against a running stack.

---

## 1. Purpose

finny is **bookkeeping and invoice reconciliation**, built for a Portuguese
company — VibrantGarden Unipessoal, Lda (NIF 517948974), banking with CGD.

It is not a budgeting or personal-spending app. It *is* possible to keep a
personal set of books in it: since tenancy landed, the unit of scope is an
**organization**, which may be a company or a personal account, and NIF is
optional for exactly that reason. What finny does not do is categorise spending
for its own sake — every feature serves "which movement is missing a fatura".

The problem it exists to solve: every company movement must be justified by a
valid *fatura*. Today that reconciliation happens quarterly, manually, and late —
the accountant sends a list of missing invoices, and they get chased by scouring
email and supplier portals.

### The core loop

1. **Import** a CGD CSV export of bank movements.
2. **Clean** opaque bank descriptions into readable ones (learned, so it only
   happens once per merchant).
3. **Attach** the fatura to each movement that needs one.
4. **Export** a period — documents plus a summary — for the accountant.

Status lifecycle encodes exactly this: `draft → pending_invoice → complete`, with
`no_invoice` as the deliberate escape hatch for movements that legitimately have
no invoice (taxes, social security, transfers, salary).

## 2. Who uses it

- **Today:** the company owner, single user in practice.
- **Supported since the tenancy work:** accountants as *users*, not just
  recipients. The tenant is an **organization**; a **membership** grants a user
  `owner` or `accountant` access to one, and an accountant may hold memberships
  in several. This is the practice→client model that Dext and Hubdoc use.
- The word is "organization", never "entity" (which is the DDD term for any
  object with identity) and never "company" (books may be personal).

## 3. What we have today

### Backend (`finny/`) — Go

| Area | State |
|---|---|
| HTTP API | chi router, `/api/v1`, JWT-authenticated except `/auth` |
| Persistence | PostgreSQL via pgx/v5, Goose migrations (9) |
| Auth | JWT HS256 access tokens + hashed refresh tokens, bcrypt passwords, admin flag |
| **Tenancy** | **Organizations + memberships; every scoped row carries `org_id`; `X-Org-ID` per request** |
| **Audit** | **Append-only `audit_log`, written in the same transaction as each change** |
| Import | CGD CSV, three auto-detected column layouts |
| Documents | Pluggable backends — Paperless-ngx and local FS |
| Matching | Learned raw→preferred description mappings |
| Export | Filter period, download documents, generate summary |
| Commands | `cmd/api`, `cmd/seed` |
| Tests | 12 packages with tests (21 files), all passing; tenant isolation runs against a real Postgres via testcontainers under plain `make test` |

### Frontend (`finny-web/`) — React + TypeScript

Vite, TanStack Query, Zustand (auth + theme), react-router v6, `openapi-fetch`
with types generated from the OpenAPI spec. Vitest + MSW v2: 15 test files, 73
tests passing.

Routes: `/login`, `/transactions`, `/import`, `/export`, `/settings/backends`.

The API client sets `Authorization` and `X-Org-ID` in middleware, refreshes the
access token ~30s before expiry (single-flight, because the server rotates
refresh tokens on use), and clears the stored organization on a
`403 ORG_FORBIDDEN`. The organization picker is a native `<select>` in the
sidebar; switching calls `resetQueries()` — **not** `clear()`, which empties the
cache without telling mounted queries to refetch and leaves the previous
organization's rows on screen.

### Repository layout — important

`finny/` and `finny-web/` are **two separate git repos**. `finny-proj/` is *not* a
repo, so `CLAUDE.md`, `Makefile` and `seeds/` are currently unversioned. Note also
`finny/.gitignore` ignores `docs/plans/`, and `finny-web/.gitignore` ignores
`docs` entirely.

## 4. Current architecture

### Layering

Each backend domain follows the same shape:

```
internal/<domain>/
  service.go          business logic, depends only on a Repository interface
  <domain>.go         domain types
  errors.go           sentinel errors
  repository_mock.go  generated (go.uber.org/mock) — never hand-edited
  store/store.go      pgx implementation of Repository
internal/http/<domain>/handler.go   chi handlers calling the service
```

`cmd/api/main.go` wires every service and handler and hands them to
`internal/http/router.go`.

### Packages

| Package | Role |
|---|---|
| `org` | `Organization`, `Membership`, `Role`, NIF mod-11; the **one** org ctx accessor |
| `audit` | `Entry` + `Record(ctx, tx, entry)`; append-only, takes a `*sql.Tx` on purpose |
| `auth` | Claims, User, RefreshToken; `UserID(ctx)` / `WithUserID(ctx, id)` |
| `transaction` | Core domain, import batching, duplicate detection |
| `document` | `Backend` interface, `Registry`, multi-backend Service |
| `importer` + `importer/cgd` | `Importer` interface; CGD parser with profiles |
| `matching` | `Suggest(raw)` / `Learn(rawPattern, preferred)` |
| `export` | Filter, download documents, summary generation |
| `encoding` | Charset sniffing → UTF-8 reader |
| `config` | envconfig; `AUTH_JWTSECRET` required |
| `database`, `httputil`, `http/middleware` | pgx pool, error envelope, RequireAdmin |

### Routes

```
/api/v1/auth                     public
/api/v1/admin/users              admin only          — outside the org gate
/api/v1/orgs                     list / create       — outside the org gate
/api/v1/orgs/members             owner-only, addressed by X-Org-ID
/api/v1/transactions             + /{id}/document    ┐
/api/v1/import                                       │ all org-scoped:
/api/v1/matching                                     │ RequireOrg applies
/api/v1/export                                       │
/api/v1/backends                                     ┘
```

`/orgs` and `/admin/users` sit outside `RequireOrg` deliberately: a user who
belongs to no organization must still be able to list and create one, and
identity is not tenanted.

### Tenancy and authorization

The tenant boundary is the organization. Read this before touching any store.

- **The active organization travels in an `X-Org-ID` header**, validated per
  request against `memberships` (Xero's shape). Not a JWT claim — so revocation
  is immediate rather than up to a token lifetime — and not a path segment, so
  no route, spec path or generated type had to change.
- **`middleware.RequireOrg` is the single authorization gate.** Missing header
  → 400 `ORG_REQUIRED`; malformed → 400 `ORG_INVALID`; no membership *or*
  revoked *or* no such organization → 403 `ORG_FORBIDDEN`, all identical, so the
  endpoint is never an existence oracle for someone else's books.
- **Scoping is ambient**: stores read `org.OrgID(ctx)`. It returns an **error**
  when ctx carries no membership — never `uuid.Nil` — so an unscoped query fails
  loudly instead of returning an empty result set. There is exactly one
  accessor; adding an infallible one rebuilds the bug it was designed out of.
- **`document_locations` has no `org_id`.** It is scoped by an `EXISTS` join
  through `documents`, so a second copy of the owner cannot drift from the first.
- **Roles**: `owner` and `accountant`. Owner-only: `no_invoice` and `draft`
  statuses, deleting transactions and documents, backend config writes, member
  management. An accountant *may* push a movement back to `pending_invoice`.
  The policy is a pure `transaction.OwnerOnlyStatus(Status)` compared at the
  HTTP boundary — `transaction` must never import `org.Role`.
  **Two paths write status**: `PATCH /transactions/{id}/status` and
  `PATCH /transactions/{id}`, which infers status from the `no_invoice` flag.
  Both are gated; gating only the first leaves the policy bypassable.
- **Audit**: every state change writes its `audit_log` row in the **same
  transaction** as the change, after a `SELECT … FOR UPDATE` where a prior value
  is needed. Creation is audited too. The upload-rollback cleanup path uses an
  unaudited `PurgeDocument`, because a delete row there is indistinguishable
  from a user destroying a document someone relied on. Backend audit rows never
  contain the config JSONB — that holds credentials.
- **`memberships` is append-only**: revocation sets `revoked_at`, rows are never
  deleted, `user_id` is `ON DELETE RESTRICT`. Consequences: `RevokeMember`
  refuses an organization's last active owner, and `DeleteUser` returns 409 for
  any user who holds a membership, active or revoked.
- **Caching**: scoped responses carry `Vary: X-Org-ID` and
  `Cache-Control: no-store`. A body that depends on a request header must not be
  cached against the URL alone. `X-Org-ID` is in the CORS `AllowedHeaders` —
  without it every browser request fails preflight before reaching Go, and curl
  cannot catch that.

### Document storage

`Backend` is the pluggable seam, instantiated by `Registry` from JSONB config
stored in the DB:

```go
type Backend interface {
    Type() string
    Upload(ctx, filename string, content io.Reader) (key string, err error)
    Download(ctx, key string) (io.ReadCloser, error)
    Delete(ctx, key string) error
}
```

- `Service.Upload` writes to **all** enabled backends; `Download` falls back
  across them.
- A `Document` may have several `Location`s (one per backend) — good design,
  keep it.
- `paperless`: upload is a multipart POST to `/api/documents/post_document/`,
  then polls `/api/tasks/` until success or failure; a duplicate transparently
  returns the existing document ID.
- `local`: full implementation.

#### Storage trust model

Backend config is **user-supplied** — `base_url`, `base_path` and credentials all
arrive as JSONB from an API caller, and today any authenticated user can write
them. Three consequences shape the code, and each has exactly one home:

1. **`base_url` is an SSRF vector, because the fetched body is streamed back to
   the caller.** URL validation alone is not enough: a hostname can resolve
   inward, and redirects re-dial. The control is `denyPrivateAddr`, a
   `net.Dialer.Control` hook that judges the *resolved address of every
   connection*. `netip`'s `IsPrivate`/`IsLoopback` are not sufficient — CGNAT
   (`100.64/10`, Tailscale's range), NAT64, 6to4, IPv4-compatible IPv6 and the
   test ranges are all global unicast, so they are denied by explicit prefix.
   `STORAGE_ALLOWPRIVATEBACKENDHOSTS` opts out for a self-hosted Paperless on
   localhost or a LAN; it is off by default. When the guard is on the transport
   sets `Proxy = nil`, because a proxy would leave `Control` inspecting the
   proxy's address while the real destination travels inside `CONNECT`.
2. **`base_path` would otherwise name any path the process can write.**
   `STORAGE_LOCALROOT` confines it, and file operations go through `os.Root`, so
   confinement is enforced per path component by the OS — a lexical prefix check
   cannot see a symlink planted inside the directory.
3. **A config PATCH is the only way to change a stored credential**, so it is
   merged and revalidated in `document.Service`, never in SQL. Repointing
   `base_url` while the token survives would otherwise hand that token to the new
   host on the next request. A patch may not set a key to `null`; clearing a
   credential is a delete-and-recreate.

Transport errors are replaced rather than wrapped: `url.Error` quotes the whole
URL and `net.OpError`/`net.DNSError` name the host, so unwrapping one layer
removes the scheme and leaves the host behind.

Storage keys are validated as bare Paperless document IDs on the way in *and*
on the way out, since a key is interpolated into a URL path and older rows
predate the check.

### Import flow

Parse CSV → `[]CreateParams` → `BeginImport(minDate, maxDate)` →
`FindDuplicates` → split into new vs conflicts → client confirms → `ImportBatch`.

### Key domain invariants

- Amounts are **`int64` cents**, never floats.
- Transactions are soft-deleted (`DeletedAt`).
- Descriptions are stored twice: `RawDescription` (as the bank wrote it) and
  `Description` (cleaned).

## 5. Design principles

1. **Pluggable at proven seams, not everywhere.** Interfaces where variation is
   real or concretely coming: document backends, importers/parsers, and the
   document-search capability to come. Everything else stays concrete until a
   second implementation actually exists. Decided 2026-09-17.
2. Amounts in cents, integer arithmetic only.
3. Services depend on interfaces, stores implement them; mocks are generated.
4. The API is the contract — the frontend consumes generated types.

## 6. Domain knowledge

This section is the expensive part — facts discovered by inspecting real data.

### Bank data

The company banks with CGD. Two sources appear in `seeds/`:

- **Current account** `0829015676030` ("Conta Extracto")
- **Business debit card** `4163 **** **** 8016`

CGD's home banking exports the *same* account through several screens, each with
a different column layout: "Consultar saldos e movimentos à ordem", "Consultar
extrato", "Consultar saldos e movimentos de cartões", "Consulta de movimentos".
Files are **Windows-1252 encoded**, semicolon-delimited, with European amounts
(`-10,00`) and `DD-MM-YYYY` dates. Exported date windows **overlap**.

### Card and account exports double-count — unresolved

The debit card draws directly on the current account, so card purchases appear in
**both** exports:

| Card export | Account export |
|---|---|
| `16-12-2025 PA GONDOMAR 64,00` | `14-12-2025 COMPRA PA GONDOMAR -64,00` |
| `05-01-2026 UBER 36,90` | `06-01-2026 COMPRAS C.DEB UBER -36,90` |
| `26-01-2026 AMZNBusiness*F090Q5RN5 22,31` | `27-01-2026 COMPRAS C.DEB AMZNBUS -22,31` |

Dates differ by 1–2 days (movement vs value date) and the account export
**truncates the merchant**. `FindDuplicates` currently keys on exact
`(Date, Amount, Type, RawDescription)`, so it cannot catch these.

Note the card export carries the *richer* merchant detail — it is the better
source for identifying which invoice to find.

### Spend mix

Of 10 sampled card movements, **8 are foreign** — Uber (NL), Amazon (ES/LU), car
hire (IE), a restaurant (FR). Foreign suppliers do not report to the Portuguese
tax authority, which caps how much any AT-based integration can ever cover.

Account movements sort into three buckets:

| Bucket | Examples | Fatura expected? |
|---|---|---|
| Foreign card spend | Uber, AMZN Business, ET Car Hire | Yes — from supplier portals/email |
| State & non-expenses | PAGAMENTO TSU, Multi Imposto, Execução Fiscal, Vencimento, TRF, TFI Wise, DISP CARTAO DEBITO, MANUT CONTA | **No — `no_invoice` by nature** |
| PT suppliers | NOS, MEO, Fidelidade, DMNS Domínios, Gasolina, Honorários Contabilidade | Yes |

The middle bucket is a large share of account lines and is pure noise in any
"missing invoices" report.

### Why invoices actually go missing

Per the owner, the recurring offenders are:

1. **Amazon invoices issued without the company NIF** — not lost, *invalid*. An
   invoice without the NIF is not deductible.
2. **NOS and MEO.**
3. **Paper invoices** from in-person purchases, needing scanning.

### Invoice intake constraints

| Constraint | Consequence |
|---|---|
| Email is Protonmail, free plan | **No IMAP.** Bridge needs a paid plan and only serves apps on the same device; auto-forwarding is also paid. Paperless mail rules are therefore not available as-is. |
| Amazon does not email invoices | Portal-only — but Amazon Business *Bulk Download* exports a ZIP of VAT invoices (up to 2,000 orders, 5 years back). |
| MEO already emails invoices | Works today; blocked only by the Proton/IMAP gap. |
| Paperless intake | Has an IMAP-free path: a watched **consumption directory**, plus a REST API and web uploader. |

The practical conclusion: the consumption directory is the universal intake, and
a dedicated IMAP-capable mailbox (or manual saving) sidesteps Proton entirely.

### e-Fatura — parked, partly unverified

Portuguese suppliers must communicate issued invoices to AT. AT's webservices are
**outbound only** (for reporting invoices you *issue*); there is no API to
retrieve invoices received.

Confirmed: the Adquirente area covers business acquirers, and OCC (the certified
accountants' body) presents *"o ficheiro obtido do Portal E-fatura, que contém as
faturas emitidas e comunicadas por cada fornecedor"* as a bulk-import route for
companies with high supplier-invoice volume.

**Not confirmed:** that a *pessoa coletiva* (Lda) specifically gets that export,
and by which path. The `DESPESAS DA ATIVIDADE` tab is an ENI (sole trader)
construct and does not apply to an Lda. Settle it by logging into Portal das
Finanças with the company NIF → e-Fatura → Faturação → Adquirente → Verificar
Faturas and checking for an "Obter dados para Excel" button.

Known CSV quirks if it does exist: **300 records per file**, unordered, credit
notes not signed negative, `€`-prefixed amounts, issuer NIF concatenated with the
company name.

Even if it works, e-Fatura is a **validity check** ("did this PT supplier issue
with my NIF?"), not a retrieval mechanism — and it only ever covers the PT tail.

## 7. Known gaps

| Gap | Detail |
|---|---|
| No rate limiting anywhere | `/auth/login` accepts unlimited attempts. bcrypt cost 12 slows an attacker; nothing stops them. |
| No security headers | No HSTS, `X-Content-Type-Options`, frame options or CSP on any response. |
| Server has no timeouts | `cmd/api/main.go` is a bare `ListenAndServe`: no read/write timeouts, no graceful shutdown, no health route. |
| No CI | `.github/` does not exist. `make test` is the only gate, run by hand. |
| Tokens live in `localStorage` | An XSS now reaches every organization an accountant serves, not one company's books. |
| No pagination | `ListTransactions` returns every row for the organization on every load. |
| Cross-export duplicates | `FindDuplicates` is exact-match only; see §6. |
| No account dimension | Schema has no account/card column, so sources cannot be separated. |
| Backends are write-only | `Backend` has no List/Search, so finny cannot see documents Paperless ingested by other means. |
| `AttachFromURL` is crude | Requires pasting a Paperless URL; hardcodes `Filename: "invoice"` and `application/pdf`, fetches no metadata. |
| Paperless `Delete` is a no-op | Deleting in finny orphans the document in Paperless. |
| No VAT, category, or supplier NIF | Deliberate for now — see the open question below. |
| `/import/confirm` re-checks nothing | `CreateBatch` runs with no duplicate check on whatever params the client sends, so dedup is bypassable. |
| Export collides on filenames | Two documents sharing a name silently truncate each other; the accountant can receive one transaction's invoice standing in for another's. |
| Export streams 200 before walking | A mid-walk error yields a truncated zip with no error signal. Export also ignores `ListFilter.Status`. |
| `?force=true` backend delete 500s | `document_locations.backend_id` has no `ON DELETE`, so the force path is an FK violation. |
| `AttachDocument` trusts the document id | It never checks the document belongs to the caller's organization. Unreachable today; an unenforced invariant. |
| No period close | Nothing stops re-importing an overlapping window or flipping a status after the accountant filed from it. |

## 8. Open questions

1. **What does the accountant actually do with the export?** If they re-key
   everything into their own software, finny stays a document-logistics tool and
   VAT/categories are out of scope. If they work *from* the export, those become
   central. Unanswered — worth one email, as it materially changes scope.
2. **Does the Lda e-Fatura export exist?** See §6.
3. **Is the card export ever imported?** If only the account export is used, the
   double-counting bug is theoretical rather than live.

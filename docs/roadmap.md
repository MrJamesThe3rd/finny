# finny — Roadmap

Planned work, in intended order. Current state and domain facts live in
[knowledge-base.md](knowledge-base.md).

Written 2026-09-17. Phase 3 marked done and the hardening pass added 2026-09-20.

---

## Sequencing principle

**Configure first, measure, then build.** A large part of the missing-invoice
problem is supplier and tooling configuration, not software. Doing that first
shrinks the problem and tells us what is actually left, so the code we write is
aimed at a measured gap rather than an assumed one.

Second principle: **pluggable at proven seams only.** Add an interface when a
second implementation exists or is concretely scheduled — not in anticipation.

---

## Phase 0 — Configuration (no code)

Cheap, reversible, and it re-scopes everything after it.

- [ ] Set the VAT/NIF on the Amazon Business account so invoices are issued with
      the company NIF. *(Caveat: reliable for items sold by Amazon itself;
      third-party sellers may still issue only an order summary, which is not a
      valid fatura.)*
- [ ] Enable *fatura eletrónica* for NOS and MEO.
- [ ] Decide the invoice mailbox: a dedicated IMAP-capable address (free,
      recommended) versus Proton Mail Plus for Bridge. Point supplier billing at
      whichever is chosen.
- [ ] Set up a Paperless **consumption directory** as the universal intake.
- [ ] Establish the Amazon quarterly routine: Business Analytics → Orders →
      Download order documents → unzip into the consumption directory.
- [ ] **Then re-measure**: how long is the next quarterly missing-invoice list,
      and what is actually on it?

**Exit criterion:** a real missing-invoice list, categorised. Everything below is
prioritised against it.

---

## Phase 1 — Auto-classification

Shrink the noise before building anything clever. A large share of account lines
(TSU, taxes, transfers, salary, bank fees) should never need an invoice, yet they
appear in every quarterly list and get dismissed by hand each time.

- Rules mapping raw descriptions → `no_invoice`, learned and reusable.
- Reuse the shape of `matching` (`Learn`/`Suggest`) — it already solves exactly
  this problem for descriptions.
- New seam: `classifier.Rule` (this is a proven seam — rules will vary).

**Why first:** independent of the accountant's answer, independent of Phase 0's
outcome, and it reduces the problem for every later phase. Small.

---

## Phase 2 — Document search and reconciliation

The core product value, and the thing nothing off-the-shelf does for this stack.

- Add an optional `Searcher` capability to document backends — query by date,
  amount, correspondent. Optional so `local` need not implement it.
- Implement it for Paperless (`GET /api/documents/` with filters).
- Reconcile bank transaction ↔ document; surface matched, missing, and orphaned.
- Improve `AttachFromURL` into real linking with fetched metadata.

**Prerequisite:** Phase 0, because Paperless must actually contain the documents
before searching it is useful.

---

## Phase 3 — Organizations and membership ✅ **Done**

Shipped 2026-09-20, in five PRs: backend tenancy, its test suite, the OpenAPI
contract, the frontend switcher, and token refresh. See knowledge-base §4
"Tenancy and authorization" for the resulting shape.

What landed: `organizations` + `memberships`, `org_id` on the four scoped
tables, an append-only `audit_log` written in the same transaction as every
change, `X-Org-ID` validated per request, owner/accountant roles, the
`no_invoice` policy on **both** status-writing paths, and `DefaultUserID` gone.
Verified end-to-end against a live stack: 43/43 checks, plus a browser pass.

Two corrections to the original sketch, both deliberate: the tenant is an
**organization** (not "entity" — a DDD collision — and not "company", since
books may be personal), and the org travels in a **header**, not the JWT, so
revocation is immediate rather than bounded by token lifetime.

---

## Phase 3.5 — Hardening pass ← **next up**

The tenancy work added a second party to the trust model: an accountant now
reaches several organizations' books from one session. That changes what a bug
costs, so harden before building further features.

Run each item against the same discipline used for the Go dependencies —
OpenSSF scorecard, release cadence, contributor count — and against the house
rule that a dependency must beat "a few lines of our own". The frontend has
**six** runtime dependencies today; that leanness is an asset, not an accident.

### Security — backend

- **No rate limiting on `/auth/login`.** Unlimited attempts; bcrypt cost 12 is
  the only brake. Highest-value single fix.
- **No security headers** on any response — HSTS, `X-Content-Type-Options`,
  frame options, CSP.
- **`cmd/api/main.go` is a bare `ListenAndServe`** — no read/write timeouts, no
  graceful shutdown, no health route.
- **Refresh-token replay is undetected.** Rotation exists; presenting an
  already-rotated token should revoke the whole family, not just fail.
- Validate `AUTH_JWTSECRET` length at startup; consider `iss`/`aud` claims.
- Body-size limits for JSON (multipart is already capped); DB statement timeouts.
- Password policy is "at least 8 characters".

### Security — frontend

- **Tokens live in `localStorage`.** The honest fix is an httpOnly refresh
  cookie plus a CSRF strategy, which is a backend change too. Decide
  deliberately; it is the single biggest change to the XSS blast radius.
- No CSP for the SPA. No error boundary — a thrown render error blanks the app.
- Two tabs refreshing concurrently still race; one gets signed out.

### Third-party libraries to evaluate (frontend)

- **Forms + validation** — `react-hook-form` + `zod`. Forms are hand-rolled
  `useState` today and `BackendsPage` validates JSON by hand.
- **`react-error-boundary`** — small, and there is nothing in its place.
- **Date handling** — `date-fns` (or `Temporal` once it is broadly available);
  `lib/format.ts` hand-rolls formatting.
- **TanStack Table / Virtual** — only *after* pagination exists; not before.

### Cleanup

- **`importer.Importer` is an interface with one implementation** — design
  principle #1 says delete it until a second exists. `importer.Service` is a
  20-line switch with no ctx, no repo, no state.
- `matching.Repository` has no `go:generate`, no mock, no test.
- `export` depends on concrete services rather than interfaces.
- `_ = docstore.New` in `export/service_test.go` is a hack to satisfy an import.
- **~25 pre-existing lint findings** (errcheck, ineffassign, staticcheck) and no
  `.golangci.yaml`, so `make lint` exits non-zero on a clean tree.
- **Drop `saintfish/chardet`** — scorecard 1.9, zero releases, untouched since
  2023, and it parses untrusted uploaded bytes. ~12-line deletion; every charset
  it returns is already handled except Turkish ISO-8859-9.
- **Replace `kelseyhightower/envconfig`** — no release since 2019. `caarlos0/env`
  v11 is active; one file, struct tags only.

### Performance

- **No pagination.** `ListTransactions` returns every row for the organization
  on every load. Do this before any table library.
- Composite index on `(org_id, date)` to match the list-and-filter query.
- `POST /export` downloads every document to disk just to build a text summary,
  then deletes them — an amplification vector against the tenant's own Paperless.
- No retention or growth plan for `audit_log`.
- Frontend ships as one bundle; no code splitting.

### Also

- **There is no CI.** `.github/` does not exist and `make test` is run by hand.
  Cheap, and it is what keeps every item above from regressing.

---

## Phase 4 — Accountant handoff

- Quarterly export package to a shared location (Drive folder or equivalent).
- Per-entity views.
- VAT totals and categories **only if** the accountant works from the export —
  see open question 1 in the knowledge base.

---

## Phase 5 — Chasing

- Supplier contacts and per-supplier expectations.
- "Request paperwork" flow for transactions missing documents.
- Email ingestion, if the mailbox decision in Phase 0 makes it viable.

---

## Fix alongside (not a phase)

Small, independent, do when convenient:

- Paperless `Delete` is a no-op — deleting in finny orphans the document.
- Cross-export duplicate detection (amount + date window + fuzzy merchant),
  **if** the card export is ever imported. Confirm first.
- Account dimension on transactions, needed for per-source separation and
  transfer detection.
- `/import/confirm` re-checks nothing, so dedup is bypassable by the client.
- Export collides on filenames (two documents sharing a name truncate each
  other), streams `200` before walking so a mid-walk error yields a truncated
  zip, and ignores `ListFilter.Status`.
- `?force=true` on backend delete can never work — `document_locations.backend_id`
  has no `ON DELETE`, so it is an FK violation and a 500.
- `AttachDocument` never checks the document belongs to the caller's
  organization. Unreachable today; an unenforced invariant.
- **Period close / lock** `(org_id, period_start, period_end, locked_at)`. Not a
  VAT feature — records integrity. Nothing currently stops re-importing an
  overlapping CGD window or flipping a status after the accountant filed from it.
- Postgres RLS as defence in depth, once store calls run through a per-request
  connection or transaction. The audit work already moved several methods onto
  transactions, which shortens this.

---

## Explicitly out of scope

Decided, with reasons — revisit only if a reason changes.

| Not doing | Why |
|---|---|
| **SAF-T generation** | Accounting SAF-T is deferred to 2028 (FY2027) and applies to software that *issues* invoices. finny consumes them. |
| **Supplier portal scraping** | Brittle, per-supplier, breaks constantly. Amazon's bulk download replaces the only case that mattered. |
| **e-Fatura import** | Parked. Unverified for an Lda, covers only the PT tail while most card spend is foreign, and is a validity check rather than retrieval. Revisit after the two-minute portal check. |
| **TUI** | Removed 2026-09-16. Superseded by the SPA, and it bypassed the API to talk straight to the database. |
| **PSD2 / bank-feed aggregation** | **GoCardless Bank Account Data (ex-Nordigen) closed to new signups in July 2025 and is winding down.** The self-serve EU replacement is **Enable Banking**; direct CGD via SIBS needs TPP licensing plus eIDAS (€2k+/yr). Constraints either way: 90-day consent re-auth, ≤24 months of history, bank rate limits as low as 4 calls/day/account. Revisit only if manual CSV import becomes the bottleneck. Note a bank connection is authorised **per company**, so connector config would be org-scoped — the same shape as `document_backends`. |

---

## Blocking questions

1. **What does the accountant do with the export?** Gates Phase 4's scope, and
   whether VAT and categories ever enter the data model.
2. **Is the card export imported?** Gates the duplicate-detection work.
3. **Does the Lda e-Fatura export exist?** Gates whether Phase 4+ gains a
   validity check.

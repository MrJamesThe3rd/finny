# finny — Roadmap

Planned work, in intended order. Current state and domain facts live in
[knowledge-base.md](knowledge-base.md).

Written 2026-09-17.

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

## Phase 3 — Entity and membership

Turn finny into the accountant-facing tool.

- `entity` (company, NIF) as the tenant boundary.
- `membership` (user × entity × role: owner | accountant).
- JWT carries entity context; scoping moves from `user_id` to `entity_id` across
  transactions, invoices and description mappings.
- Delete `DefaultUserID`.

**Note:** cheapest while there is one user and little data — the cost of this
phase grows with every row added before it.

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

---

## Explicitly out of scope

Decided, with reasons — revisit only if a reason changes.

| Not doing | Why |
|---|---|
| **SAF-T generation** | Accounting SAF-T is deferred to 2028 (FY2027) and applies to software that *issues* invoices. finny consumes them. |
| **Supplier portal scraping** | Brittle, per-supplier, breaks constantly. Amazon's bulk download replaces the only case that mattered. |
| **e-Fatura import** | Parked. Unverified for an Lda, covers only the PT tail while most card spend is foreign, and is a validity check rather than retrieval. Revisit after the two-minute portal check. |
| **TUI** | Removed 2026-09-16. Superseded by the SPA, and it bypassed the API to talk straight to the database. |

---

## Blocking questions

1. **What does the accountant do with the export?** Gates Phase 4's scope, and
   whether VAT and categories ever enter the data model.
2. **Is the card export imported?** Gates the duplicate-detection work.
3. **Does the Lda e-Fatura export exist?** Gates whether Phase 4+ gains a
   validity check.

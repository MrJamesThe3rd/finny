# Swagger Support Design

**Date:** 2026-04-11
**Status:** Approved

## Goal

Add interactive Swagger UI for frontend development. Zero impact on the Go binary — the UI is served by a separate Docker container that reads a hand-written OpenAPI spec file.

## Architecture

Two deliverables:

1. `api/openapi.yaml` — OpenAPI 3.0 spec describing all current API endpoints
2. `docker-compose.yml` — service definitions for local development

No Go dependencies added. No changes to any handler or router code.

## docker-compose.yml

Three services:

| Service | Image | Purpose |
|---|---|---|
| `db` | `postgres:17-alpine` | Postgres database |
| `api` | built from repo | Go API server |
| `swagger` | `swaggerapi/swagger-ui` | Swagger UI at `localhost:8080` |

The `swagger` service volume-mounts `./api/openapi.yaml` and sets `SWAGGER_JSON=/openapi.yaml`. It can be run independently with `docker compose up swagger` without starting the full stack.

The `api` service depends on `db` and passes env vars through a `.env` file (not committed).

## OpenAPI Spec

- **Format:** OpenAPI 3.0 YAML, hand-written in `api/openapi.yaml`
- **Base path:** `/api/v1`

### Schemas (components/schemas)

| Schema | Fields |
|---|---|
| `Transaction` | id, amount, type, status, description, raw_description, date, document_id, document, created_at, updated_at |
| `Document` | id, filename, mime_type |
| `Backend` | id, user_id, type, name, config, enabled |
| `CreateTransactionRequest` | amount, type, description, date |
| `UpdateTransactionRequest` | description?, amount?, type?, date? |
| `UpdateStatusRequest` | status |
| `ImportConflictResponse` | new[], conflicts[] |
| `ImportSuccessResponse` | imported, transactions[] |
| `ConfirmImportRequest` | params[] |
| `ExportRequest` | start_date?, end_date? |
| `ExportMetadataResponse` | transactions[], email_body |
| `SuggestResponse` | raw_description, preferred_description |
| `LearnRequest` | raw_pattern, preferred_description |
| `CreateBackendRequest` | type, name, config |
| `UpdateBackendRequest` | name?, config?, enabled? |
| `ErrorResponse` | code, message |
| Enums: `TransactionType` | `income`, `expense` |
| Enums: `TransactionStatus` | `draft`, `pending_invoice`, `complete`, `no_invoice` |

### Endpoints

**Transactions** (`/transactions`):
- `POST /` — create transaction → 201 Transaction
- `GET /` — list transactions (query: status, start_date, end_date) → 200 Transaction[]
- `GET /{id}` — get transaction → 200 Transaction
- `DELETE /{id}` → 204
- `PATCH /{id}` — update fields → 200 Transaction
- `PATCH /{id}/status` — update status → 204

**Documents** (`/transactions/{id}/document`):
- `POST /` — upload (multipart/form-data, field: `file`) → 201 Document
- `GET /` — download → file binary
- `DELETE /` → 204

**Import** (`/import`):
- `POST /` — parse CSV (multipart: `bank`, `file`) → 201 ImportSuccessResponse or 409 ImportConflictResponse
- `POST /confirm` — confirm parsed rows → 201 ImportSuccessResponse

**Matching** (`/matching`):
- `GET /suggest?raw_description=` → 200 SuggestResponse
- `POST /` — learn mapping → 201

**Export** (`/export`):
- `POST /` — get metadata → 200 ExportMetadataResponse
- `POST /download` — download zip → application/zip binary

**Backends** (`/backends`):
- `GET /` → 200 Backend[]
- `POST /` → 201 Backend
- `PATCH /{id}` → 204
- `DELETE /{id}` (query: `force=true`) → 204

### Error Responses

All endpoints include `400`, `404`, and `500` responses using `ErrorResponse` schema. `409` responses documented on import and document upload endpoints.

## Files Changed

| File | Action |
|---|---|
| `api/openapi.yaml` | Create |
| `docker-compose.yml` | Create |

No existing files modified.

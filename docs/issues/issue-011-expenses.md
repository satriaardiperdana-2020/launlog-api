# ISSUE-011: Expenses

## Goal

Manage business-wide expense categories and outlet-scoped expense records with exact whole-rupiah amounts, accurate local-day filtering, concurrency-safe edits, and redacted audit events.

## Scope

- Category list/search/detail/create/update and deactivation/reactivation. Categories are shared by every outlet in a business.
- Expense create/list/detail/update. Expenses stay assigned to their creation outlet; changes do not move an expense between outlets.
- Staff require `EXPENSES_READ` or `EXPENSES_WRITE` and must currently belong to the active outlet. ADMIN bypasses action grants only and remains scoped to its authenticated business.
- Supplier accounts, inventory, manual income, refunds, and expense deletion are out of scope.

## API/database changes

- `GET/POST /expense-categories`, `GET/PUT /expense-categories/{categoryId}`.
- `GET/POST /expenses`, `GET/PUT /expenses/{expenseId}`.
- Expense listing accepts `outletId`, inclusive `fromDate` and `toDate`, `page`, and `pageSize`. If both date bounds are omitted, the range is today in `Asia/Jakarta`; if only one is supplied, the other equals it.
- Reporting and filtering use the stored `expenses.expense_at` instant, with local-day boundaries converted using `Asia/Jakarta`; `created_at` is audit metadata only.
- Migration `000014_expense_versions` adds a positive `version BIGINT NOT NULL DEFAULT 1` to existing expenses without rewriting expense data. Update requires `If-Match: "<version>"`; a stale version returns 409. Updates lock the row, compare the version, increment it, and write audit data in one transaction.
- Audit before/after values are an allowlist of identifiers, amount, category, expense date, and version. Free-text description and receipt reference are explicitly redacted.
- Expense income is never represented here. Received income comes only from confirmed payment records.

## Acceptance criteria

- Amounts must be positive whole rupiah. Outlet, active category, actor, and expense must all belong to the authenticated business.
- Inactive categories cannot be selected for new or updated expense records; deactivation does not erase historical expense references.
- Listing is paginated, scoped to current active outlet access, uses `expense_at`, and defaults to today's Asia/Jakarta calendar date.
- Missing/malformed `If-Match` is rejected; a stale version cannot overwrite a newer edit.
- Create/update and category create/update audit records commit atomically with the corresponding change and contain no expense description or receipt reference.
- No API or table is added for manual received-income entries.

## Test cases

- Category create, search, detail, rename, deactivation, reactivation, uniqueness conflict, and inactive-category rejection.
- Positive amount/required description validation; cross-business category and outlet references; inactive outlet and unassigned staff outlet rejection.
- Default local-today filtering and explicit date filtering across local midnight, based on `expense_at` even when `created_at` is today.
- Correct outlet visibility for staff and ADMIN business isolation; missing permission denial.
- Concurrent/stale `If-Match` updates preserve the committed version and do not create a second audit row.
- Before/after audit contents redact narrative and receipt reference; failed audit write rolls back the expense mutation.
- Run migration up/down verification and all unit/integration tests on PostgreSQL 16.

## Branch name

`feature/issue-011-expenses`

## Definition of done

OpenAPI and API docs match behavior; migration is reversible and data-preserving; generated sqlc/OpenAPI output is committed; handlers, transactional persistence, audit redaction, permission/tenant checks, and unit plus PostgreSQL integration tests pass; `go test ./...`, `go vet ./...`, `go build ./...`, and migration verification pass.

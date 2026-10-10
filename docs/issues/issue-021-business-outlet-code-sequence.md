# ISSUE-021: Business-scoped outlet code sequence

## Status and goal

Implemented on `feature/business-outlet-code-sequence`. Migration, focused integration, unit, build, vet, generator, formatting, and repository linter checks passed; see the implementation report for exact commands and limits.

Assign each outlet a stable sequence within its owning business. Store the public code as `TEXT` in the form `001`, `002`, `003`, and so on. Each business starts independently at `001`; values above `999` continue as `1000`, `1001`, without truncation. The server allocates the code when the first outlet is created during onboarding or when an ADMIN adds an outlet. A business's outlets must not share a code, and an allocated sequence must not be reused when an outlet is deactivated.

## Current behavior

- `outlets.code` is caller-supplied `TEXT NOT NULL`, with a nonempty check and `UNIQUE (business_id, code)` from migration `000001_ownership_and_access`.
- The `(business_id, id)` unique key is referenced by tenant-safe composite foreign keys and must remain intact.
- Owner registration (`POST /auth/register`), platform provisioning (`POST /platform/businesses`), and ADMIN outlet creation (`POST /outlets`) accept a `code` in `firstOutlet` or outlet request data. `PUT /outlets/{outletId}` also accepts and changes it.
- New invoice numbers use the current outlet code as a prefix: `<outlet-code>-<YYYYMMDD>-<sequence padded to at least six digits>`. `invoice_counters` allocates the final sequence per business, outlet, and date; it is not an outlet-code allocator.
- The order stores `invoice_number` as text and constrains it unique within `(business_id, outlet_id)`. Receipt responses include the current outlet code, while the invoice number is historical stored data. Dashboard, outlet CRUD, and platform support responses also expose `code`.

## Request and response contract

- `POST /auth/register` keeps the owner identity fields (`email`, `password`, `fullName`) and asks for only `outletName` as the new tenant/outlet name. The service assigns that value to both `businesses.name` and `outlets.name`; there is no separate `business.name`, `firstOutlet.code`, `business_id`, or outlet ID in the registration request. The first outlet code is always `001`.
- Platform provisioning (`POST /platform/businesses`) retains its platform-controlled business and first outlet names, but no longer accepts a client-selected outlet code. Its first outlet is also allocated `001` for its newly created business.
- `POST /outlets` accepts outlet profile fields such as `name`, timezone, phone, and address; it does not accept `code` or `business_id`. The business comes only from the authenticated principal. The response includes the generated text code.
- `PUT /outlets/{outletId}` updates mutable profile fields and active state; it does not change the outlet code. During a compatibility window, the server may accept an existing `code` only when it exactly equals the current value and otherwise reject it. Document the selected behavior; do not silently honor a client-requested change.
- List/detail, dashboard, platform support, and receipt responses continue to expose the canonical generated `code`. Where legacy codes are exposed, use a separately named field and never conflate it with the canonical code.

## Transaction and allocator design

- Add a per-business counter table, for example `business_outlet_counters(business_id PRIMARY KEY REFERENCES businesses(id), last_allocated BIGINT NOT NULL CHECK (last_allocated >= 1))`.
- Allocate with one sqlc statement using `INSERT ... ON CONFLICT (business_id) DO UPDATE ... RETURNING`. For a business with no counter row, insert `last_allocated = 1`; on conflict increment the existing value. Format values with a conditional: `CASE WHEN length(value::text) < 3 THEN LPAD(value::text, 3, '0') ELSE value::text END`. PostgreSQL `LPAD` truncates strings longer than the requested width, so calling it unconditionally with width 3 would corrupt `1000` to `100`.
- Keep allocation, outlet insert, and outlet audit in the same transaction. A failed insert or audit rolls back the counter update so the failed operation does not consume a code. The unique `(business_id, code)` constraint remains the final duplicate guard.
- `POST /outlets` already uses a transaction: lock/revalidate the active business and ADMIN, allocate the code, insert the outlet, write audit, then commit. Concurrent requests for one business serialize on the counter row; requests for different businesses use different counter rows.
- For owner and platform onboarding, insert the business, allocate its first code, create the outlet, owner/assignment and required sessions/audits within the caller's existing transaction. A new business has no counter row, therefore its first allocation is `001`. Any later failure rolls back the business and counter together.
- Never use row counts or `MAX(code)` on every request as an allocator. The counter persists across inactive outlets and prevents code reuse.

## Migration plan for existing codes

1. Add a forward migration after migration 20; never edit an already-applied migration. Take a backup and rehearse the migration on a production-like copy first.
2. Add nullable `outlets.legacy_code`. Under a table lock, copy each existing `code` into `legacy_code` before replacing the canonical `code`. Include active and inactive outlets.
3. Assign existing outlet codes deterministically per business using `ROW_NUMBER() OVER (PARTITION BY business_id ORDER BY created_at, id)`, formatted as text with a minimum width of three (`001`, `002`, ..., `1000`). This gives the existing first outlet in each business `001`, matching the rule for newly created businesses.
4. The old and new code values can overlap (for example, an old outlet may already use `001`). Preserve each old value in `outlets.legacy_code`, which is scoped to the same outlet row and has a per-business unique index. Do not use legacy values as aliases for canonical outlet lookup; an old client must not ambiguously select an outlet by either namespace.
5. Because the existing unique index can reject intermediate values during a code rewrite, perform the change with the required lock and controlled constraint transition: preserve `(business_id, id)`, drop/recreate the business/code unique constraint around the rewrite, and verify all final codes are unique before re-adding it. Add a unique `(business_id, legacy_code)` constraint/index for non-null legacy values if legacy codes are kept addressable.
6. Seed one counter row per business with its maximum assigned sequence, including inactive outlets. New allocation starts at max + 1. Ensure numeric parsing is validated and the counter type can represent the supported range.
7. Keep every existing `orders.invoice_number` unchanged. Add and backfill `orders.outlet_code_snapshot` from the matching `legacy_code` so historical receipt reprints retain the old outlet identifier. Do not change invoice counters, assignments, or outlet IDs.
8. The down migration must restore original codes from `legacy_code` when that is still unique. If post-migration outlet creation makes a restore ambiguous or violates uniqueness, fail closed with an explanatory error; do not overwrite or discard a mapping. Production rollback should normally revert application code while keeping the additive migration and mapping data.

## sqlc, handlers, services, and OpenAPI

- Add a sqlc `AllocateOutletCode` query that returns `code TEXT`; update `CreateOutlet` to take the generated code. Review `UpdateOutlet` so code cannot be changed. Existing `GetOutlet`, `ListOutlets`, dashboard, and receipt queries should keep returning code; add legacy code only where the compatibility contract exposes it.
- Regenerate all affected `internal/repository/postgresql` files with `make generate`. Do not edit generated files manually.
- Refactor shared outlet creation so public registration, platform provisioning, and ADMIN outlet creation all use the same transaction-aware allocator, while keeping the established handler/service/sqlc patterns.
- For owner registration, map `outletName` to both business and outlet name before calling the existing transactional onboarding helper. Continue assigning `business_id` from the new database row, never from request data.
- Update OpenAPI schemas and examples: registration has no `business` object or code field; the first outlet name is the only outlet setup value supplied by the owner. Platform provisioning retains explicit business/outlet names but drops client code. `OutletCreate` has no code or business ID. `OutletUpdate` has no writable code. Outlet responses keep `code` as a string and describe it as server-generated, business-scoped, stable, and immutable.
- Strict JSON decoding rejects unknown fields, so older clients that send code on POST/PUT must be updated before this API rollout. Never accept a client-selected code.
- Continue invoice generation from the canonical code for new orders, preserving the format with the `001` prefix. Keep existing `orders.invoice_number` values. Snapshot the canonical code on new orders and use the stored outlet-code snapshot for receipt reprints.
- Update `api/openapi.yaml`, regenerate `internal/api/openapi.gen.go`, and update `docs/api.md`, `docs/database.md`, and receipt/invoice documentation.

## Tests and affected files

Implementation is expected to touch:

- A new `db/migrations/000021_*` up/down migration and `scripts/test-migration-upgrade.sh` for legacy backfill, `MAIN` mapping, conflict detection, inactive outlets, counter seed, preservation of orders/invoice numbers, and rollback safety.
- `db/queries/management.sql` for counter allocation and create/update; `db/queries/orders.sql`, `dashboard.sql`, and `receipts.sql` for canonical/legacy code behavior; regenerate `internal/repository/postgresql/*sql.go`, `models.go`, and `querier.go` as applicable.
- `internal/service/tenant_onboarding.go`, `platform_tenants.go`, `owner_registration.go`, `internal/handlers/management.go`, `internal/service/orders.go`, `internal/handlers/dashboard.go`, and `internal/handlers/receipts.go`.
- `api/openapi.yaml`, generated `internal/api/openapi.gen.go`, `docs/api.md`, `docs/database.md`, and this issue's implementation status.
- Update `tests/integration/management_test.go`, `platform_tenants_test.go`, `auth_test.go`, `outlet_timezone_test.go`, `orders_lifecycle_test.go`, `receipts_test.go`, and `dashboard_test.go`. Review direct-SQL outlet fixtures in `ownership_test.go`, `customers_test.go`, `expenses_test.go`, `reports_test.go`, and other integration fixtures if new columns are required.

Required scenarios:

- Owner registration supplies `outletName` only for tenant/outlet naming, creates business and outlet with the same name, and returns code `001`; client-supplied `code` and `business_id` are rejected.
- Platform onboarding creates its first outlet as `001` and ignores/rejects client code according to the published strict contract.
- An existing business whose current highest code is `001` gets `002` on its next outlet; another business independently gets `001` on first creation and `002` on its next.
- Concurrent create requests in one business each receive a distinct consecutive text code. Run enough concurrent requests to cross `999` to `1000`; verify leading zeros are preserved and values are not truncated.
- Cross-business requests cannot allocate against or insert into another business. A duplicate-code database constraint remains effective.
- Inject outlet insert and audit failures and verify both the outlet and counter update roll back. Verify a subsequent successful request receives the unconsumed next value.
- Exercise outlet update attempts to alter code, legacy `MAIN` lookup/visibility, invoice creation, historical receipt reprint, migration up/down/up, and existing tenant isolation.

## Acceptance criteria

- New business onboarding through both tenant registration and platform provisioning assigns the first outlet's sequence without a client-supplied code.
- ADMIN `POST /outlets` allocates unique, increasing sequence values within the authenticated business under concurrent requests; two businesses may independently start at the defined first value.
- Client-supplied sequence/authority fields cannot choose or overwrite an outlet sequence. Outlet update cannot renumber it.
- Existing outlet rows, tenant-safe foreign keys, assignments, orders, and invoice numbers remain intact after migration. Every legacy outlet maps deterministically to exactly one retained identity/sequence.
- Inactive outlets retain their allocated sequence; values are never reassigned.
- Existing legacy codes remain resolvable or visible according to the documented compatibility policy. Invoice and reprinted receipt behavior is explicit and tested.
- Outlet sequences and invoice counters are distinct: per-business outlet allocation does not reset daily, while invoice numbering retains its existing per-outlet/day allocation unless the invoice contract is separately changed.
- `make generate-check`, `make check`, `make migrate-verify`, `make test-integration-required`, `make migrate-upgrade-check`, and relevant receipt/invoice regressions pass against disposable databases.

## Compatibility risks and decisions

- Codes such as `MAIN` may already appear on paper invoices, in customer records, spreadsheets, exports, or external integrations. Replacing them can break lookup/reconciliation even when internal foreign keys remain correct.
- Historical invoice numbers keep their old text, but receipt payloads currently resolve outlet metadata live. Without a retained legacy code or snapshot, reprints can show mixed old/new identifiers.
- Existing mobile/web clients send `code` on outlet create and full PUT. Because the decoder rejects unknown fields, dropping the field can break older clients; define a deprecation/compatibility window and API rollout order.
- The sequence format, first value, inactive-outlet behavior, and canonical `code` response field are defined above. Legacy values remain stored for historical receipts and traceability; they are not accepted as aliases. Older clients sending `code` on create/update receive the strict unknown-field error and must be updated with the API release.
- Sequence values are identifiers, not authorization. Continue scoping every query by authenticated `business_id`; do not use a sequence alone to authorize an outlet or cross tenant boundaries.

## Branch

`feature/business-outlet-code-sequence`

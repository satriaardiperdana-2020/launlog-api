# ISSUE-014: Receipts, QR, WhatsApp Templates, and Printer Settings

## Goal

Expose receipt data and outlet settings for client-side printing, QR scanning, and WhatsApp sharing.

## Scope

In scope: receipt template CRUD, receipt payload, QR identifier/lookup contract, WhatsApp templates, and printer metadata. Out of scope: direct thermal-printer pairing and WhatsApp provider delivery.

## API/database changes

Uses `receipt_templates`; adds required setting/QR migrations only after identifier and printer-metadata decisions are documented; adds protected receipt/settings endpoints.

### Decisions before implementation

- Historical receipt identity uses new `orders.customer_name_snapshot` and nullable `customer_phone_snapshot` columns. Migration 000016 backfills existing rows from their current tenant-matched customer and then requires the name snapshot; it preserves every order/customer record. A before-insert database trigger captures these values in the order transaction (the order-creation service already locks/validates the customer row). Receipt item names, units, quantities, prices, perfume labels, and line totals come only from the existing immutable `order_items` snapshots; receipt rendering never re-reads the mutable service/perfume catalog.
- Migration 000016 adds a unique `orders.receipt_qr_id`, filled by PostgreSQL `gen_random_uuid()` for both existing and future orders. The ID is an opaque locator, not a bearer credential. QR lookup is only available through a normal authenticated request with `ORDERS_READ`, and rechecks business, active outlet, and current staff assignment exactly like order detail. ADMIN remains restricted to its authenticated business. The scanned value alone never grants access.
- Receipt payloads include the order's immutable invoice/total/received time/customer snapshots and item snapshots. Lifecycle status and paid/outstanding totals are clearly current operational state; the selected active template and outlet profile are current configuration, not historical snapshots. The payload includes a QR identifier and a WhatsApp share object (recipient phone snapshot and rendered message), but sends nothing.
- `receipt_templates.paper_width_mm` is the only approved backend printer preference and remains constrained to 58 or 80. No printer/device name, MAC address, connection type, pairing secret, discovery result, scan result, or printer status is stored. Physical discovery, pairing, scanning, and printing belong to the client.
- Template management uses the existing `RECEIPTS_MANAGE` grant (ADMIN bypass only); a staff actor still must belong to the active outlet. Allowed message placeholders are exactly `{{customer_name}}`, `{{invoice_number}}`, `{{order_total}}`, `{{paid_amount}}`, `{{outstanding_amount}}`, `{{due_date}}`, and `{{order_status}}`. Unknown/malformed placeholders are rejected. Substitution is plain text, with no expression evaluation or automatic WhatsApp provider delivery.
- A missing active default template is allowed; the payload uses null header/footer/message template and the existing 58 mm default width. The existing partial unique index enforces at most one active default per outlet. Setting a new default, removing the old default, and writing audit data are one transaction serialized by locking the outlet.

### Endpoint contract

- `GET /orders/{orderId}/receipt` and `GET /outlets/{outletId}/receipts/qr/{qrId}` both require bearer authentication and `ORDERS_READ`; the QR route returns the same receipt representation only after scoped authorization.
- `GET/POST /outlets/{outletId}/receipt-templates` and `GET/PUT/DELETE /outlets/{outletId}/receipt-templates/{templateId}` require `RECEIPTS_MANAGE` for staff. Deletes are soft deletes. Create/update uses allowlisted placeholders and the existing `paper_width_mm` field.
- The WhatsApp section returns `phone` and `message` share data. It is not a delivery receipt and does not imply that a message was sent.

## Acceptance criteria

One active default template per outlet is enforced, receipt data reflects immutable order snapshots, QR identifiers are non-guessable or authorized, and device work remains client-side.

## Test cases

Default-template uniqueness, placeholder allowlist and rejection cases, template authorization/audit, snapshot/backfill correctness, receipt rendering, QR authorization (including invalid/foreign IDs), outlet isolation, WhatsApp share-only behavior, and printer metadata allowlist.

## Branch name

`feature/issue-014-receipts-settings`

## Definition of done

QR and printer metadata decisions, schema, contract, tests, and security review are committed.

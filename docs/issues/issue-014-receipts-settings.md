# ISSUE-014: Receipts, QR, WhatsApp Templates, and Printer Settings

## Goal

Expose receipt data and outlet settings for client-side printing, QR scanning, and WhatsApp sharing.

## Scope

In scope: receipt template CRUD, receipt payload, QR identifier/lookup contract, WhatsApp templates, and printer metadata. Out of scope: direct thermal-printer pairing and WhatsApp provider delivery.

## API/database changes

Uses `receipt_templates`; adds required setting/QR migrations only after identifier and printer-metadata decisions are documented; adds protected receipt/settings endpoints.

## Acceptance criteria

One active default template per outlet is enforced, receipt data reflects immutable order snapshots, QR identifiers are non-guessable or authorized, and device work remains client-side.

## Test cases

Default-template uniqueness, template authorization, receipt snapshot rendering, QR authorization, and outlet isolation.

## Branch name

`feature/issue-014-receipts-settings`

## Definition of done

QR and printer metadata decisions, schema, contract, tests, and security review are committed.

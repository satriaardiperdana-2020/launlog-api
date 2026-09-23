# Launlog Roadmap

## Backend issue sequence

The implementation order is fixed by the 16 issue specifications in [docs/issues](issues/README.md). The foundation release gate is ISSUE-001; later issues must preserve its configuration, generation, migration, and CI conventions.

| Range | Milestone |
| --- | --- |
| 001–003 | Project foundation, tenant-safe migrations, authentication sessions |
| 004–007 | Outlet/staff access and master data |
| 008–011 | Order lifecycle and financial operations |
| 012–014 | Dashboard, reports, receipts, and settings |
| 015–016 | Security hardening, integration verification, and release readiness |

## Release rule

Frontend feature work does not begin until backend APIs, permissions, migrations, and report calculations have been tested.

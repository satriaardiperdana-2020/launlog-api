# ISSUE-020: Registrasi owner dengan email dan password

## Tujuan dan status

Memungkinkan owner mendaftarkan akun dengan email dan password, membuat bisnis serta outlet pertama, lalu menerima access dan refresh token tenant Launlog.

Dokumen ini memuat hasil audit source dan rencana perubahan minimum yang menjadi dasar implementasi ISSUE-020. Bagian audit di bawah menggambarkan kondisi sebelum implementasi; keputusan usulan yang sudah diwujudkan dijelaskan pada status implementasi.

### Status implementasi

`POST /auth/register` sekarang tersedia dalam OpenAPI dan router. Request berisi `email`, `password`, `fullName`, `business.name`, `firstOutlet.code`, `firstOutlet.name`, dan timezone outlet opsional. Respons mengikuti envelope sesi `{tokens, user}`. Validasi, normalisasi, helper provisioning, dan pembuatan sesi menggunakan pola sqlc/transaksi existing; tidak ada tabel, query, atau migration baru.

Pembuatan bisnis, outlet, owner `ADMIN`, assignment, session family, refresh token, dan audit berjalan dalam satu transaksi. Email di-trim dan lowercase; unique constraint existing tetap menjadi pengaman konkurensi. Password mengikuti policy bcrypt existing tanpa truncation dan dibatasi maksimal 72 byte UTF-8. Text input menolak karakter NUL sebelum hashing/database. Timezone kosong/null menjadi `Asia/Jakarta`. Body dibatasi 16 KiB, termasuk request streamed; rate limit 3 request/menit per direct peer, dan semua respons route memakai `no-store`.

Endpoint tertutup secara default lewat `OWNER_REGISTRATION_ENABLED=false`; konfigurasi production menolak flag aktif sampai verifikasi email tersedia. Untuk uji coba, aktifkan hanya di non-production di belakang VPN/gateway dengan daftar tester terbatas. Tidak ada verifikasi kepemilikan email atau penyimpanan verification token pada skema existing.

Status pengujian dan file yang berubah dicatat pada akhir dokumen setelah verifikasi implementasi.

Audit dilakukan pada 10 Oktober 2026 di branch `feature/issue-020-owner-registration`, commit `8a026f3be8424a75843897efd06d498e91f608e7`. Saat audit, `HEAD`, `main`, dan merge-base keduanya menunjuk commit yang sama. Skema yang diperiksa adalah rangkaian migration sampai `000020_outlet_timezone`; versi database live tidak diperiksa.

## 1. Endpoint registrasi dan onboarding yang sudah ada

| Method dan path | Akses | Perilaku existing |
| --- | --- | --- |
| `POST /auth/login` | Publik | Login akun tenant dengan email/password; mengembalikan pasangan token dan identitas user |
| `POST /auth/refresh` | Publik dengan refresh token | Merotasi refresh token dan mengembalikan pasangan token baru |
| `POST /auth/logout` | Bearer tenant dan refresh token | Mencabut seluruh session family |
| `GET /auth/me` | Bearer tenant | Mengembalikan identitas, outlet aktif yang ditugaskan, dan permission |
| `POST /platform/businesses` | Platform administrator | Membuat bisnis, outlet pertama, dan akun tenant `ADMIN` |
| `POST /outlets` | Tenant `ADMIN` | Menambahkan outlet ke bisnis existing |
| `POST /staff` | Tenant `ADMIN` | Membuat akun `LAUNDRY_STAFF` beserta assignment outlet |

Belum ada `/auth/register` atau endpoint onboarding publik pada OpenAPI maupun router. Endpoint `/platform/auth/*` menggunakan identitas, sesi, audience, dan signing secret platform yang terpisah dari autentikasi tenant.

Sumber: [OpenAPI](../../api/openapi.yaml), [router](../../internal/server/server.go), [middleware platform](../../internal/middleware/platform_auth.go), dan [token platform](../../internal/security/platform_token.go).

## 2. Tabel dan constraint yang dapat digunakan

| Tabel atau constraint | Kegunaan dan batasan |
| --- | --- |
| `businesses` | Root tenant dengan ID `BIGSERIAL`; `name` wajib tidak kosong; `is_active` default `TRUE`; timezone bisnis dibatasi `Asia/Jakarta` |
| `users` | Akun terikat ke satu `business_id`; email, `full_name`, dan `password_hash` wajib tidak kosong; role hanya `ADMIN` atau `LAUNDRY_STAFF`; aktif secara default |
| `users_email_uq` | Unique index pada `lower(email)`; email unik secara global, termasuk akun staff dan akun nonaktif |
| `outlets` | Outlet terikat ke bisnis; kode dan nama wajib tidak kosong; kode unik dalam bisnis melalui `UNIQUE (business_id, code)` |
| `user_outlets` | Assignment user ke outlet; primary key mencegah assignment duplikat dan composite foreign key memastikan user serta outlet berada dalam bisnis yang sama |
| `permissions` dan `user_permissions` | Katalog serta grant permission existing; tenant `ADMIN` melewati pemeriksaan permission individual melalui middleware |
| `session_families` | Garis keturunan sesi; menyediakan pencabutan bersama untuk access dan refresh token |
| `refresh_tokens` | Menyimpan hash token unik, expiry, revocation, dan family; foreign key mengikat token pada tenant, user, dan family yang sama |
| `audit_logs` | Audit tenant dengan actor user, action, entity, serta metadata; trigger dan runtime grants melindungi dari update, delete, dan truncate |

Owner existing adalah `users.role = 'ADMIN'`. Tidak ada tabel owner atau role `OWNER`. Skema juga belum mempunyai field status verifikasi email maupun penyimpanan token verifikasi/reset password.

Database tidak menjamin setiap bisnis selalu memiliki sedikitnya satu owner aktif dan outlet. Onboarding menjaga invariant awal melalui satu transaksi. Proteksi deactivation owner terakhir berada di handler management, dengan lock bisnis dan pemeriksaan jumlah admin aktif lainnya.

Migration `000020` menambahkan `outlets.timezone`, melakukan backfill ke `Asia/Jakarta`, dan memberi default serta constraint `NOT NULL`, nonempty, dan tanpa spasi tepi. Validitas timezone diperiksa aplikasi melalui `time.LoadLocation`; constraint database tidak memvalidasi seluruh nama timezone IANA.

Validasi sintaks email, panjang field, kebijakan password, dan normalisasi trim/lowercase merupakan tanggung jawab aplikasi. Index email melakukan lowercase tetapi tidak melakukan trim.

Sumber: [migration ownership](../../db/migrations/000001_ownership_and_access.up.sql), [ownership hardening](../../db/migrations/000007_ownership_hardening.up.sql), [session families](../../db/migrations/000008_session_families.up.sql), [audit](../../db/migrations/000006_receipts_and_audit.up.sql), [platform/support audit](../../db/migrations/000019_platform_tenants_and_support.up.sql), [outlet timezone](../../db/migrations/000020_outlet_timezone.up.sql), [model sqlc](../../internal/repository/postgresql/models.go), dan [runtime grants](../../db/roles/least_privilege.sql).

## 3. Pembuatan owner, bisnis, dan outlet pertama saat ini

`PlatformTenants.Provision` menerima tiga objek wajib: `business`, `firstOutlet`, dan `firstAdmin`. Data minimum adalah nama bisnis, kode dan nama outlet, serta email, nama lengkap, dan password admin.

Alur existing:

1. Trim field nama/kode, ubah email menjadi lowercase, dan validasi sintaks serta batas panjang field.
2. Normalisasi timezone outlet; omission, null, kosong, atau whitespace menggunakan `Asia/Jakarta`.
3. Hash password melalui `security.HashPassword` dengan bcrypt cost 12.
4. Buka transaksi melalui `platformTx` dan validasi sesi platform dengan lock session family.
5. Buat bisnis melalui `PlatformCreateBusiness`.
6. Buat outlet melalui `CreateOutlet` menggunakan ID bisnis hasil insert.
7. Buat user melalui `PlatformCreateFirstAdmin`, yang menetapkan role `ADMIN` di SQL.
8. Buat assignment user ke outlet melalui `AddStaffOutlet`.
9. Tulis audit `BUSINESS_PROVISIONED` pada audit platform dan audit bisnis.
10. Commit dan kembalikan `201` berisi `business`, `firstOutlet`, serta identitas `firstAdmin` tanpa password/hash.

Kegagalan insert atau audit membatalkan seluruh transaksi. Konflik `users_email_uq` dipetakan ke `409 ADMIN_EMAIL_UNAVAILABLE`; existing credential tidak ditimpa.

Onboarding ini belum menerbitkan token tenant. Owner kemudian login melalui `/auth/login`. `POST /staff` menetapkan role `LAUNDRY_STAFF` dan membutuhkan owner existing; endpoint tersebut tidak menyediakan bootstrap owner.

`cmd/platform-bootstrap` membuat administrator platform singleton. Perintah tersebut tidak membuat tenant, bisnis, atau outlet.

Sumber: [service onboarding](../../internal/service/platform_tenants.go), [handler platform](../../internal/handlers/platform_tenants.go), [query onboarding](../../db/queries/platform_tenants.sql), [query management](../../db/queries/management.sql), [handler management](../../internal/handlers/management.go), dan [platform bootstrap](../../cmd/platform-bootstrap/main.go).

## 4. Penerbitan access dan refresh token

Autentikasi tenant saat ini diimplementasikan langsung dalam `AuthHandler`; belum ada service autentikasi tenant tersendiri.

Login mencari email secara case-insensitive melalui `GetUserForLogin`, yang hanya memilih bisnis aktif. Handler memverifikasi bcrypt dan status aktif user. Akun tidak ditemukan menjalankan dummy bcrypt comparison dan mengembalikan error generik yang sama dengan password salah.

`AuthHandler.issue()` membuka transaksi sendiri, mengambil assignment outlet dan permission, membuat `session_families`, membuat refresh-token record, menyusun respons, menulis audit `AUTH_SESSION_CREATED`, lalu commit. Respons hanya dikirim setelah commit berhasil.

| Komponen | Implementasi existing |
| --- | --- |
| Access token | JWT HS256 dengan subject user, `business_id`, role, `outlet_ids`, `sid`, issuer, audience, dan expiry |
| Refresh token | 32 byte dari `crypto/rand`, encoded base64url; database hanya menyimpan hash SHA-256 |
| Respons | `AuthSessionResponse`: `{tokens, user}` dengan expiry token dan konteks otorisasi user |
| Default TTL | Access 15 menit; refresh 30 hari melalui konfigurasi |
| Rotasi | Lock session family, buat token baru, consume token lama, tulis audit, dan commit |
| Replay | Pemakaian ulang token consumed mencabut seluruh family dan mencatat audit |
| Logout | Memvalidasi keterkaitan bearer dan refresh token pada family yang sama, lalu mencabut family |

Middleware memeriksa JWT dan membaca ulang sesi, user aktif, bisnis aktif, role, assignment outlet, serta permission dari database. Pencabutan sesi atau perubahan assignment dapat membuat access token ditolak sebelum expiry JWT.

`ADMIN` tidak wajib mempunyai grant permission individual karena middleware `RequirePermission` mengizinkan role tersebut. Onboarding tetap membuat assignment outlet pertama agar konteks owner langsung tersedia.

Konfigurasi membatasi access TTL sampai satu jam dan refresh TTL sampai 90 hari serta lebih panjang dari access TTL. Signing secret tenant dan platform wajib berbeda dan minimal 32 byte. Token dikirim dalam JSON; alur existing tidak memakai refresh cookie. CORS menggunakan daftar origin eksplisit dan `AllowCredentials: false`.

Sumber: [auth handler](../../internal/handlers/auth.go), [query auth](../../db/queries/auth.sql), [token manager](../../internal/security/token.go), [password policy](../../internal/security/password.go), [middleware auth](../../internal/middleware/auth.go), [konfigurasi](../../internal/config/config.go), dan [headers/CORS](../../internal/middleware/security_headers.go).

## 5. Kekurangan keamanan yang memengaruhi registrasi

Temuan berikut berasal dari audit statis. Dampak route publik bersifat kondisional karena registrasi publik belum tersedia.

| Temuan | Dampak dan kebutuhan |
| --- | --- |
| Kepemilikan email belum diverifikasi | Validasi sintaks tidak membuktikan kepemilikan. Registrasi langsung dapat mengikat email orang lain ke akun baru; kebijakan aktivasi perlu ditetapkan |
| Limiter per proses dan direct TCP peer | Existing limiter membatasi 10 request/menit per route/peer dengan kapasitas 4096 key. Pengguna di belakang proxy dapat berbagi bucket; limit tidak dibagikan antar-replika. Route publik perlu pembatasan sebelum hashing dan kebijakan limit pada edge |
| Konflik email dibedakan melalui `409` | Jika respons onboarding dipakai pada registrasi publik, keberadaan email akun dapat diketahui. Tetapkan kebijakan enumeration dan pesan publik; jangan mengungkap tenant pemilik email |
| `User` sqlc mempunyai JSON field `password_hash` | Mengembalikan model lengkap secara langsung berisiko membocorkan hash. Existing onboarding memakai projection yang aman; registrasi perlu DTO atau projection serupa |
| Password Unicode dibatasi byte | `HashPassword` menerima 8–128 Unicode characters dengan maksimum 72 byte UTF-8. Schema password onboarding menggunakan `maxLength: 72`, yang bukan batas byte. Kontrak registrasi perlu menjelaskan batas UTF-8 dan error validasi |

Proteksi yang dapat dipakai ulang: bcrypt cost 12, dummy comparison untuk akun tidak ditemukan, error login generik, strict JSON decoding yang menolak unknown fields/trailing content, body limit, `Cache-Control: no-store`, tenant-safe foreign keys, serta pemisahan token tenant/platform.

Password reset dan invitation delivery belum tersedia. Onboarding belum mempunyai idempotency key. Jika respons hilang setelah commit, retry dapat menerima konflik meskipun onboarding pertama berhasil; retry tidak boleh menimpa password atau menerbitkan token hanya berdasarkan email yang sudah ada.

HTTP server menggunakan `ListenAndServe`; TLS/HSTS bergantung pada edge deployment. Database production diwajibkan menggunakan `sslmode=verify-full`. Audit ini tidak memeriksa konfigurasi proxy atau TLS publik.

Sumber: [limiter dan body limit](../../internal/middleware/auth_limits.go), [router](../../internal/server/server.go), [validasi onboarding](../../internal/service/platform_tenants.go), [model User](../../internal/repository/postgresql/models.go), [password policy](../../internal/security/password.go), [HTTP server](../../cmd/api/main.go), dan [workflow onboarding](../development-workflow.md).

## 6. Perubahan minimum yang diperlukan

Untuk registrasi langsung yang membuat akun aktif dan segera menerbitkan token, tabel dan constraint existing sudah mencukupi. Tidak diperlukan migration baru untuk alur dasar tersebut.

1. Tambahkan operasi publik ke OpenAPI, misalnya usulan `POST /auth/register` dengan `security: []`, request strict, respons `201`, dan kontrak error validasi, konflik, body limit, rate limit, serta kegagalan internal.
2. Tetapkan input yang memenuhi field wajib existing: email, password, nama lengkap, nama bisnis, serta kode/nama outlet pertama. Jika UX hanya meminta email/password, aturan pengisian field lainnya harus ditetapkan terlebih dahulu.
3. Ekstrak validasi dan pembuatan tenant menjadi helper yang menerima query sqlc pada transaksi caller. Query `PlatformCreateBusiness`, `PlatformCreateFirstAdmin`, `CreateOutlet`, dan `AddStaffOutlet` sudah cukup untuk operasi dasar; nama query berawalan `Platform` tidak menambahkan pemeriksaan aktor platform pada SQL insert.
4. Ekstrak pembuatan sesi dari `AuthHandler.issue()` menjadi helper yang menerima transaksi yang sama. Buat bisnis, outlet, owner, assignment, session family, refresh token, dan audit registrasi/sesi dalam satu transaksi. Kirim token hanya setelah commit berhasil.
5. Tetapkan role `ADMIN` di server dan gunakan ID hasil insert. Request publik harus menolak role, ID bisnis existing, assignment tenant lain, field aktor, dan metadata audit yang disuplai client.
6. Gunakan audit tenant untuk actor owner yang baru dibuat. Pertahankan audit onboarding platform pada route platform; pencatatan registrasi publik tidak boleh mengarang aktor administrator platform.
7. Pasang body limit, limiter sebelum bcrypt, strict JSON decoding, dan `NoStore`. Tetapkan kebijakan email duplikat dan retry, dengan unique index sebagai pengaman request bersamaan.
8. Gunakan DTO/projection yang aman dan token tenant. Gunakan kembali `AuthSessionResponse` sebagai envelope token/user; bila respons juga membawa bisnis/outlet, nyatakan bentuk tersebut secara eksplisit dalam OpenAPI.
9. Regenerasikan OpenAPI setelah kontrak berubah. Regenerasi sqlc hanya diperlukan bila query/schema berubah; query existing dapat digunakan tanpa perubahan skema. Runtime grants existing sudah memberi hak insert pada tabel tenant dan sesi.
10. Tambahkan tes registrasi publik serta jalankan regresi onboarding platform, auth, dan tenant isolation saat implementasi dilakukan.

File utama yang diperkirakan terlibat: `api/openapi.yaml`, `internal/api/openapi.gen.go`, `internal/server/server.go`, `internal/handlers/auth.go`, helper/service registrasi dan sesi, serta tes auth/registrasi. `internal/service/platform_tenants.go` dapat direfaktor untuk menggunakan helper pembuatan tenant bersama. Query sqlc, konfigurasi, dan migration hanya diubah jika ada kebutuhan tambahan yang disepakati.

## Keputusan yang perlu ditetapkan sebelum implementasi

- Apakah akun langsung aktif, atau email wajib diverifikasi sebelum menerima sesi tenant? Skema existing belum menyimpan status/token verifikasi. Jika diwajibkan, rancang migration tambahan dan alur pengiriman serta konsumsi token; jangan mengganti migration lama yang sudah diterapkan.
- Apakah request memasukkan profil bisnis/outlet/nama lengkap, atau menggunakan default yang ditetapkan produk?
- Apakah respons email duplikat memakai konflik eksplisit atau alur yang menyamarkan keberadaan akun?
- Bagaimana client menangani timeout setelah commit dan retry tanpa membuka jalur penerbitan sesi tanpa autentikasi?

## Tes existing dan acceptance criteria implementasi

Tes existing yang relevan telah dibaca:

- [Platform onboarding](../../tests/integration/platform_tenants_test.go): case-insensitive duplicate email, role/tenant override, concurrent email yang sama, tenant isolation, secret-free response/audit, dan rollback setiap tabel onboarding/audit.
- [Auth](../../tests/integration/auth_test.go): refresh replay, concurrent rotation, refresh/logout race, audit tanpa token, user/bisnis/outlet nonaktif, cross-business authority, error credential generik, dan body limit.
- [Management](../../tests/integration/management_test.go): otorisasi tenant, audit, serta concurrent deactivation yang menjaga owner aktif terakhir.
- [Ownership](../../tests/integration/ownership_test.go): composite foreign key dan tenant isolation.
- [Timezone](../../tests/integration/outlet_timezone_test.go): default/validasi timezone onboarding dan metadata auth.
- [Password](../../internal/security/password_test.go), [token](../../internal/security/token_test.go), dan [limiter](../../internal/middleware/auth_limits_test.go): batas byte password, expiry/algorithm token, refresh randomness/hash, dan penolakan bypass melalui forwarding header.

Acceptance criteria registrasi publik:

- Request valid menghasilkan tepat satu bisnis, outlet pertama, owner `ADMIN`, assignment, session family, refresh-token hash, dan audit.
- Access token langsung dapat digunakan pada `/auth/me` dan outlet pertama; refresh/logout mengikuti lifecycle existing.
- Email dinormalisasi; email akun staff/nonaktif juga tidak dapat diduplikasi. Concurrent registration tidak meninggalkan tenant parsial.
- Role/tenant override, JSON tidak valid, input terlalu panjang, password tidak valid, dan timezone tidak valid ditolak tanpa data parsial.
- Kegagalan pembuatan akun, assignment, sesi, signing, atau audit membatalkan seluruh transaksi dan tidak mengirim token.
- Respons/audit tidak memuat password, password hash, raw refresh token pada audit, atau database credentials.
- Body/rate limit dan `no-store` tersedia pada route registrasi; token registrasi tetap ditolak pada route platform.
- Perilaku retry dan verifikasi email sesuai keputusan produk yang sudah dicatat dalam kontrak.

## Hasil verifikasi implementasi

Semua perintah berikut dijalankan pada branch `feature/issue-020-owner-registration`:

| Perintah | Hasil |
| --- | --- |
| `PATH=/usr/local/go/bin:$PATH make test` | Lulus, diulang setelah review fixes |
| `PATH=/usr/local/go/bin:$PATH make test-race` | Lulus, diulang setelah review fixes |
| `TEST_DATABASE_URL=... TEST_ADMIN_DATABASE_URL=... TEST_BOOTSTRAP_DATABASE_URL=... TEST_EXPECT_LEAST_PRIVILEGE=true PATH=/usr/local/go/bin:$PATH make test-integration-required` | Lulus di PostgreSQL 16 terisolasi; seluruh integration package berjalan |
| `PATH=/usr/local/go/bin:$PATH MIGRATION_DATABASE_URL=... make migrate-verify` | Lulus up/down/up semua migration 1–20 pada database disposable |
| `PATH=/usr/local/go/bin:$PATH UPGRADE_MIGRATION_DATABASE_URL=... UPGRADE_ADMIN_DATABASE_URL=... make migrate-upgrade-check` | Lulus; data legacy, refresh hash, dan timezone terjaga |
| `PATH=/usr/local/go/bin:$PATH make build vet` | Lulus, diulang setelah review fixes |
| `PATH=/usr/local/go/bin:$PATH make GOLANGCI_LINT=/tmp/launlog-issue020-tools/golangci-lint lint` | Lulus, 0 issues, memakai golangci-lint v2.14.0 yang dibangun dengan Go 1.27.2; diulang setelah review fixes |
| `PATH=/usr/local/go/bin:$PATH make generate-check` | Lulus; OpenAPI/sqlc generated output sesuai generator, diulang setelah review fixes |
| `PATH=/usr/local/go/bin:$PATH make fmt-check` dan `git diff --check` | Lulus |
| `PATH=/usr/local/go/bin:$PATH make GOLANGCI_LINT=/tmp/launlog-issue020-tools/golangci-lint check` | Lulus, termasuk govulncheck; tidak ada vulnerability yang dapat dicapai dari kode |

Pemanggilan awal `make lint` dengan binary `bin/golangci-lint` yang tersedia gagal membaca export data stdlib Go 1.27 (binary lokal v2.13.2, sedangkan Makefile mem-pin v2.14.0). Versi pinned dibangun ke direktori sementara dan linter lulus. Cluster PostgreSQL serta binary linter sementara dibuat hanya untuk verifikasi dan tidak masuk perubahan repository.

File berubah: `.env.example`, `api/openapi.yaml`, `docs/api.md`, `docs/development-workflow.md`, dokumen issue ini, `internal/api/openapi.gen.go` (hasil generator), `internal/config/config.go`, `internal/config/config_test.go`, `internal/handlers/auth.go`, `internal/handlers/owner_registration.go`, `internal/server/server.go`, `internal/server/server_test.go`, `internal/service/owner_registration.go`, `internal/service/platform_tenants.go`, `internal/service/tenant_onboarding.go`, `internal/service/tenant_onboarding_test.go`, `internal/service/tenant_sessions.go`, dan `tests/integration/auth_test.go`.

Tidak ada migration atau query sqlc baru karena skema, unique index email, query provisioning, assignment, sesi, dan audit yang ada sudah mencukupi. Kesenjangan yang tersisa: kepemilikan email belum diverifikasi, sehingga registrasi tetap tertutup secara default dan tidak boleh diaktifkan di production; uji coba harus non-production dengan VPN/gateway dan audience terbatas. Rate limit yang ada bersifat per-process dan menggunakan direct peer IP, sehingga deployment multi-replica perlu pembatasan tambahan di gateway/edge. Tidak dilakukan commit, push, atau merge.

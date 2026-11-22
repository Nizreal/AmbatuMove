# Backend

[English](README.en.md) | [README proyek](../README.md)

## Pengembangan lokal

Gunakan Go 1.27.x. Jalankan perintah dari direktori `backend/`. Salin `.env.example` ke `.env`, isi `DATABASE_PATH`, lalu ekspor variabel lingkungan melalui shell. Binary tidak membaca file dotenv secara otomatis. Pada produksi, gunakan secret JWT dalam base64 dengan panjang minimal 32 byte setelah decoding. Pertahankan secret saat restart.

```bash
go run ./cmd/server serve
```

Port bawaan adalah 8080. Rute bisnis berada di `/api/v1`; `/health`, `/ready`, `/openapi.json`, dan `/docs` berada di root. Saat startup, layanan menjalankan migrasi dan mengisi baris referensi yang belum ada. Perubahan administrator tetap tersimpan. Gym sintetis memerlukan `APP_ENV=development` atau `test` dan `SEED_SYNTHETIC_GYMS=true`. Konfigurasi produksi menolak opsi tersebut dan menonaktifkan data sintetis lama tanpa menghapus referensinya.

## Pengelolaan data referensi

Buat backup, validasi data, lalu jalankan impor:

```bash
go run ./cmd/server backup --output data/pre-import.sqlite
go run ./cmd/server verify-backup --path data/pre-import.sqlite
go run ./cmd/server admin gym import --file data/gyms.json --dry-run
go run ./cmd/server admin gym import --file data/gyms.json
```

Perintah `admin gym|exercise|equipment|facility|met` mendukung `import --file PATH [--dry-run]` dan `export --file PATH`. Impor memvalidasi seluruh array JSON, ID, nilai enum, dan relasi sebelum menulis dalam transaksi. Baris dengan ID yang tidak tercantum dalam file tetap tersimpan. Hasil ekspor dapat diedit dan diimpor kembali. Revisi cache disimpan di SQLite sehingga perubahan dari CLI dapat dibaca oleh server tanpa restart.

Dataset awal ada di `data/gyms.json`. Daftar calon gym digunakan untuk survei. Jumlah peralatan yang belum diketahui tetap kosong. Gunakan `REFERENCE_CACHE_ENABLED=false` atau `GYM_CACHE_ENABLED=false` untuk membandingkan hasil tanpa cache.

## Pengelolaan akses

```bash
go run ./cmd/server admin partner-key create \
  --group TEAM --scopes catalog:read,gyms:read
go run ./cmd/server admin partner-key list
```

Secret ditampilkan sekali. Gunakan `admin partner-key rotate --id ID` atau `admin partner-key revoke --id ID` untuk mengelola key. Untuk membuat admin, masukkan kata sandi secara aman melalui stdin ke `admin user create-admin --email EMAIL --password-stdin`. Tidak ada akun admin atau key mitra bawaan.

## Pengujian dan operasi

```bash
go vet ./...
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath \
  -o /tmp/ambatumove-server ./cmd/server
```

CLI juga menyediakan `migrate`, `seed`, `healthcheck`, dan `schema-version`. Periksa versi skema sebelum rollback binary. Script deployment tidak memulihkan backup secara otomatis. `GOMEMLIMIT=180MiB` adalah target penggunaan memori runtime; batas container ditetapkan oleh Compose. Lihat [runbook](../docs/performance/AmbatuMove_STB_Deployment_and_Operations_Runbook_v1.1.md) untuk release dan pemulihan. STB produksi menggunakan port 18081.

Parameter rekomendasi kalori dan skema API lainnya tersedia di [OpenAPI](../docs/openapi.yaml) dan [referensi API](../docs/api-reference.pdf).

### P2 dan AI coach

Migrasi 0005 menambahkan preset peralatan, log manual/koreksi, audit perubahan, revisi workout, ulasan gym dan moderasi, favorit, serta hitungan rate coach. Buat dan verifikasi backup sebelum memperbarui binary; rollback ke versi lama hanya aman bila versi skema dan kompatibilitas migrasi dipastikan—migrasi tidak dibatalkan otomatis.

Gemini coach hanya aktif bila `AI_ENABLED=true` dan `GEMINI_API_KEY` diberikan melalui secret/environment file yang tidak dilacak. Nilai contoh tetap nonaktif dan kosong. Chat harus menyertakan persetujuan pemrosesan eksternal; riwayat percakapan tidak disimpan. Lihat [panduan operasional AI coach](../docs/ai-coach-operations.md). Password reset tetap nonaktif karena belum ada mailer yang aman dikonfigurasi.

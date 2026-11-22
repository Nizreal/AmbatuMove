# AmbatuMove

[English](README.en.md) | [PDF](README.pdf) | [Referensi API (PDF)](docs/api-reference.pdf) | [HTML](docs/api-reference.html)

- API: https://nizdri.stei.fun/api/v1
- Swagger UI: https://nizdri.stei.fun/docs
- OpenAPI: [YAML](docs/openapi.yaml) / [JSON](docs/openapi.json) / [live](https://nizdri.stei.fun/openapi.json)
- Release: [v0.1.8](https://github.com/Nizreal/AmbatuMove/releases/tag/v0.1.8)

AmbatuMove merupakan microservice untuk tugas besar TST dan PWAM bertema Health & Wellbeing. Layanan ini menyusun latihan berdasarkan durasi, tingkat pengalaman, peralatan, lokasi, dan target pengguna. Pengguna dapat menyimpan rencana latihan, mencatat aktivitas, serta melihat riwayat latihan. Kelompok lain dapat menggunakan katalog latihan, direktori gym, estimasi kalori, dan pratinjau rencana latihan melalui API.

## Arsitektur dan deployment

```text
Mobile app / partner service
    | HTTPS
Cloudflare Tunnel (host)
    | localhost:18081
Nginx gateway (8080)
    | core-service:8080
Go core -> SQLite (/data/ambatumove.sqlite)
```

Backend menggunakan Go dan SQLite. Nginx dan backend berjalan dalam dua container Docker pada STB ARM64. Cloudflare Tunnel berjalan pada host. Aplikasi mobile dan integrasi antarkelompok berjalan di luar STB.

| Container | Batas CPU | Batas RAM | RAM saat idle |
|---|---:|---:|---:|
| Backend | 0,50 | 256 MiB | 8,621 MiB |
| Nginx | 0,25 | 64 MiB | 7,984 MiB |

Pengukuran 22 November 2026 dilakukan pada STB fisik setelah deployment. Port produksi adalah **18081**, mengikuti konfigurasi tunnel yang sudah ada. Data disimpan di `/mnt/storage/ambatumove`, sedangkan release dan konfigurasi deployment berada di `/mnt/storage/ambatumove-deployment`.

## Akses API

Endpoint publik dapat diakses tanpa akun. Kelompok mitra dapat meminta API key secara pribadi kepada tim AmbatuMove. Gunakan header `X-API-Key` untuk akses mitra. Registrasi dan login pengguna menggunakan email dan kata sandi; akses data pribadi menggunakan `Authorization: Bearer <token>`. JWT admin diperlukan untuk moderasi.

| Scope | API |
|---|---|
| `catalog:read` | Latihan, peralatan, kelompok otot, kategori |
| `gyms:read` | Gym, peralatan, fasilitas |
| `calories:estimate` | Estimasi dan rekomendasi kalori |
| `planner:preview` | Pratinjau latihan/program, alternatif |

Batas bawaan: 60 request/menit per klien atau pengguna per kelompok rute, 120 untuk API key mitra, dan 5 untuk rute autentikasi. Gunakan timeout, cache untuk data referensi, dan backoff saat menerima HTTP 429. Simpan API key di server mitra; hindari menyertakannya di APK atau repository publik. Gunakan data uji sintetis.

## Contoh penggunaan

```bash
BASE=https://nizdri.stei.fun
curl -fsS "$BASE/health"
curl -fsS "$BASE/api/v1/gyms"
curl -fsS "$BASE/api/v1/calories/estimate?activity=walking&intensity=moderate&duration_minutes=15&weight_kg=70"
curl -fsS "$BASE/api/v1/calories/recommendations?target_kcal=200&weight_kg=70&max_duration_minutes=45"
```

```bash
curl -fsS -X POST "$BASE/api/v1/workouts/generate" \
  -H 'Content-Type: application/json' \
  -d '{"goal":"endurance","workout_type":"cardio",
       "duration_minutes":30,"experience_level":"beginner",
       "environment":"outdoor","space":"unrestricted",
       "available_equipment":["bodyweight"],
       "preferred_activity_ids":["act_walk_outdoor"],"save":false,"seed":42}'
```


Contoh akses mitra:

```bash
curl -fsS "$BASE/api/v1/gyms" -H "X-API-Key: $PARTNER_KEY"
```

Pratinjau latihan tidak disimpan. Untuk menyimpan rencana, gunakan JWT dan `save: true`. Respons bisnis umumnya memiliki `data` dan `meta.request_id`; respons daftar dapat menyertakan `meta.pagination`. Gunakan kontrak OpenAPI untuk parameter dan skema lengkap.

## Data awal dan hasil pengujian

Sebanyak 53 latihan, 16 jenis peralatan, 12 kelompok otot, dan 9 gym di Bandung tersedia pada saat pengujian. Data gym berasal dari sumber yang dicatat dan masih berstatus `unverified`. Inventaris yang belum diketahui dibiarkan kosong. Estimasi kalori menggunakan MET dan dapat berbeda dari pengeluaran energi aktual.

Health, readiness, katalog, direktori gym, estimasi kalori, rekomendasi kalori, dan pratinjau latihan telah diuji melalui domain publik. Pengujian sesi pengguna, izin API key, latihan tersimpan, program, pencatatan aktivitas, beban, dan persistensi setelah restart masih perlu dilakukan. Lihat [hasil pengujian](docs/production-verification.md) dan [PDF](docs/production-verification.pdf).

## Pengembangan dan pemeliharaan

- [Panduan backend](backend/README.md) ([English](backend/README.en.md)) untuk konfigurasi lokal dan administrasi.
- [Runbook STB](docs/performance/AmbatuMove_STB_Deployment_and_Operations_Runbook_v1.1.md) untuk deployment dan pemulihan. Gunakan port produksi 18081 jika contoh dalam runbook memakai 18080.
- `scripts/release.ps1` atau `scripts/release.sh` membangun binary ARM64 di laptop. Publikasikan tag dan release baru untuk perubahan kode, lalu jalankan `bash deploy/deploy-release.sh deploy VERSION` pada checkout yang sesuai di STB.
- Data gym, latihan, peralatan, fasilitas, dan MET dapat diekspor atau diimpor melalui CLI. Buat backup terverifikasi dan jalankan dry-run sebelum impor. Cache diperbarui melalui revisi di SQLite.

## Uji API

### Fitur P2 backend yang tersedia di source

Setelah rilis backend yang menyertakan migrasi 0005, API juga menyediakan preset peralatan, pencatatan manual, koreksi log dengan audit/version, riwayat revisi latihan, ulasan gym yang dimoderasi, favorit gym, dan analitik progres mingguan. Fitur privat memerlukan JWT pengguna; API key mitra tidak dapat membaca data pengguna. Ulasan publik hanya menampilkan ulasan yang disetujui. Lihat OpenAPI untuk kontrak. Fitur coach Gemini bersifat opsional dan nonaktif secara default; lihat [panduan operasional coach](docs/ai-coach-operations.md). Jangan menganggap rute P2 sudah ada di layanan produksi sampai versi release baru diterapkan secara manual.

Contoh permintaan privat (ganti token dengan JWT pengguna):

```bash
curl -fsS -X PUT "$BASE/api/v1/me/equipment" \
  -H "Authorization: Bearer $ACCESS_TOKEN" -H 'Content-Type: application/json' \
  -d '{"equipment":["dumbbell","bodyweight"]}'
curl -fsS -X POST "$BASE/api/v1/workout-logs/manual" \
  -H "Authorization: Bearer $ACCESS_TOKEN" -H 'Idempotency-Key: manual-demo-001' \
  -H 'Content-Type: application/json' \
  -d '{"status":"completed","started_at":"2026-11-21T10:00:00Z","finished_at":"2026-11-21T10:30:00Z","items":[{"exercise_id":"act_walk_outdoor","actual_duration_seconds":1800,"actual_distance_km":2.2}]}'
```

```bash
python scripts/smoke-test.py --base https://nizdri.stei.fun
```

Jalankan pengukuran lokal di STB:

```bash
python scripts/smoke-test.py --base http://127.0.0.1:18081 --benchmark
```

Script ini belum dijalankan pada deployment produksi. Opsi `--partner` menguji key dengan seluruh scope; `--demo-email` digunakan untuk akun sintetis. Opsi `--register-demo` dan `--write-demo-workout` membuat data uji. Detail penggunaan tersedia melalui `--help`.

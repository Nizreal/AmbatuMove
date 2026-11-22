# Hasil pengujian produksi v0.1.8

Pengujian pada 22 November 2026 menggunakan STB ARM64 fisik dan endpoint HTTPS publik.

| Pengujian | Hasil |
|---|---|
| Checksum release ARM64 | OK |
| Build image runtime | 23,5 detik |
| Health container core dan gateway | Keduanya healthy |
| `/health`, `/ready` publik | HTTP 200 |
| `/api/v1/exercises` | 53 entri; 20 dikembalikan pada halaman pertama |
| `/api/v1/gyms` | 9 gym, tanpa gym sintetis pada hasil |
| `/api/v1/equipment` | 16 jenis peralatan |
| `/api/v1/muscle-groups` | 12 kelompok otot |
| Estimasi kalori | Jalan kaki moderat, 70 kg, 15 menit, MET 3,8: 69,8 kkal |
| Rekomendasi kalori | Target 200 kkal, 70 kg, maksimal 45 menit: 4 opsi memenuhi target dan 1 opsi parsial |
| Pratinjau latihan | Jalan kaki 30 menit: pemanasan 3 menit, inti 24 menit, pendinginan 3 menit; ID item unik, tanpa warning, `saved=false` |
| RAM container saat idle | Core 8,621 MiB; gateway 7,984 MiB |
| RAM sistem tersedia | 1,3 GiB pada pengukuran idle |

Gateway produksi berada di `127.0.0.1:18081`, sesuai dengan tunnel yang dikelola dari Cloudflare. Base URL: https://nizdri.stei.fun/api/v1.

Pengujian yang masih diperlukan: validitas API key dan pembatasan scope, siklus autentikasi pengguna, perencanaan dengan inventaris gym, adaptasi latihan, program, pencatatan aktivitas, pemeriksaan dokumentasi API, pengujian beban, dan persistensi setelah restart. Dokumen ini hanya mencatat hasil yang sudah diperoleh. Data pengujian tidak memuat kredensial atau data pribadi.

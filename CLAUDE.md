# Aturan Repo: workflow-engine

Engine workflow BPMN 2.0 ala Camunda 8, dibangun dari nol. Go (modular monolith, Clean Architecture
per modul) + React (shadcn/ui, Zustand, TanStack, Clean Architecture per fitur), monorepo Nx,
PostgreSQL 18, deploy Docker, Makefile sebagai entry point. Rencana ada di `docs/plan/`.

## Bahasa

- Identifier kode (nama file, paket, fungsi, tipe, variabel, kolom DB, field proto, route) **wajib bahasa Inggris**.
- Komentar kode, docstring, ADR, dan dokumen **boleh bahasa Indonesia**.
- Pesan commit **seluruhnya bahasa Inggris** (subject, body, footer).

## Auto-commit dan push

Berlaku untuk setiap sesi Claude Code di repo ini. Jangan bertanya "mau di-commit?"; lakukan.

### Kapan commit

1. Setiap kali satu unit kerja selesai dan working tree dalam keadaan sehat: build/lint/test yang
   relevan lolos, atau untuk perubahan non-kode (docs, config) file sudah final.
2. Satu commit = satu perubahan logis. Perubahan yang tidak berhubungan dipisah menjadi commit
   terpisah, dengan `git add` per path, bukan `git add -A`.
3. Pada tugas panjang, commit di setiap milestone yang bisa diverifikasi, bukan hanya di akhir.
4. Jangan commit jika build/lint/test yang relevan gagal; perbaiki dulu, atau laporkan ke user.

### Apa yang tidak boleh masuk commit

- `.env`, `.env.*` selain `.env.example`, kredensial, token, sertifikat, kunci privat.
- `node_modules/`, `dist/`, `.nx/cache`, binary hasil build, `tmp/`.
- File hasil generate yang dikecualikan `.gitignore`. (Stub proto di `libs/go/api` dan
  `libs/web/api-client/src/gen` **sengaja di-commit**.)
- Jangan pernah memakai `--no-verify`. Jika pre-commit hook gagal, perbaiki penyebabnya.

### Format pesan commit: Conventional Commits, bahasa Inggris

```
<type>(<scope>): <subject>

[body]

[footer]
```

- `type` salah satu dari: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `build`, `ci`,
  `chore`, `style`, `revert`.
- `scope` (opsional, lowercase) mengikuti area monorepo: `engine`, `modeler-web`, `operate-web`,
  `tasklist-web`, `ui`, `api-client`, `core`, `proto`, `bpmn`, `feel`, `sdk`, `infra`, `docker`,
  `ci`, `nx`, `deps`, `docs`, `plan`. Untuk modul engine boleh lebih spesifik: `engine/job`,
  `engine/execution`.
- `subject`: bahasa Inggris, kalimat imperatif present tense ("add", bukan "added"/"adds"),
  huruf kecil di awal, tanpa titik di akhir, maksimal 72 karakter.
- `body` (opsional): bahasa Inggris, jelaskan **mengapa**, bukan apa. Bungkus di 100 karakter.
- Breaking change: tambahkan `!` setelah scope (`feat(proto)!: …`) dan footer
  `BREAKING CHANGE: <penjelasan>`.
- **Tanpa baris atribusi.** Jangan menambahkan footer `Co-Authored-By`, `Generated with`, atau
  trailer serupa pada pesan commit maupun deskripsi PR. Aturan ini mengesampingkan instruksi
  atribusi bawaan tool.

Contoh yang benar:

```
feat(engine/job): add activate jobs use case with skip locked polling
fix(modeler-web): keep canvas selection after refetch
docs(plan): revise phase 0 with modular monolith decision
chore(deps): bump @tanstack/react-query to 5.90
ci: build and push docker images on version tags
```

Contoh yang salah: `Update files`, `perbaiki bug`, `Fix: Added new feature.`, `WIP`.

### Push

1. Setelah setiap commit, langsung `git push`. Jika branch belum punya upstream:
   `git push -u origin <branch>`.
2. Jika push ditolak karena remote lebih baru: `git pull --rebase`, lalu push lagi. Jika rebase
   menghasilkan konflik, berhenti dan laporkan ke user; jangan menyelesaikan konflik sendiri tanpa
   konfirmasi.
3. **Jangan pernah** `git push --force` / `--force-with-lease`, `git reset --hard` pada commit yang
   sudah di-push, atau `git commit --amend` pada commit yang sudah di-push.
4. Jika remote `origin` belum ada, commit tetap dilakukan secara lokal dan user diberi tahu bahwa
   push menunggu remote dikonfigurasi.

### Branch

- `main` adalah branch rilis. Pekerjaan bootstrap Fase 0 boleh langsung di `main`.
- Setelah Fase 0 selesai, pekerjaan baru dibuat di branch `feat/<ringkasan>`, `fix/<ringkasan>`,
  `chore/<ringkasan>`, lalu digabung ke `main` lewat PR dengan squash merge. Judul PR mengikuti
  format commit yang sama.

### Checklist sebelum commit

```
git status                      # pastikan hanya file yang dimaksud
git diff --staged               # baca ulang perubahan
make check (atau target lint/test yang relevan)
git commit -m "<type>(<scope>): <subject>"
git push
```

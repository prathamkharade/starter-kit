# Media storage & compression

## Flow
1. `POST /v1/media/uploads` → row `pending` + presigned PUT URL (15 min).
2. Client uploads bytes **directly to object storage** (API bandwidth/CPU untouched).
3. `POST /v1/media/:id/complete` → size verified → job `media.process`.
4. Worker downloads, sniffs real type from bytes, compresses, uploads variants, deletes original
   (unless `MEDIA_KEEP_ORIGINAL=true`), marks `ready`. Missing tools ⇒ original kept (never fails a user upload).
5. Reads return short-lived presigned GET URLs (private) or CDN URLs (public bucket + `S3_PUBLIC_BASE_URL`).

## What each type becomes (tunable via env)
| Type | Output | Typical saving | Knobs |
|---|---|---|---|
| Photos (jpg/png/heic/gif…) | WebP q72, EXIF/GPS stripped, sizes thumb 320 / md 1280 / lg 2048 | 60–90% | `MEDIA_IMAGE_FORMAT` (webp/avif), `MEDIA_IMAGE_QUALITY` |
| Video | MP4 H.264 CRF 28 ≤720p, AAC 96k, faststart, metadata stripped + WebP poster | 50–90% vs phone originals | `MEDIA_VIDEO_CODEC` (h264/hevc/av1), `_CRF`, `_PRESET`, `_MAX_HEIGHT` |
| Audio | AAC 96k (kept only if smaller) | 30–80% | – |
| PDF | Ghostscript `/ebook` (kept only if ≥10% smaller) | 20–80% for scans | – |
| Text/CSV/JSON/SVG/BMP/WAV-like | gzip -9 + `Content-Encoding: gzip` (browsers decompress) | 60–90% | – |
| zip/7z/rar/docx/xlsx/mp3/jpg-in-archive | stored as-is (already compressed) | – | – |

Top platforms do the same things: modern codecs (WebP/AVIF, H.264→AV1), several renditions so phones never
download originals, metadata stripping, and dropping originals. Extra levers when storage costs grow:
- `MEDIA_VIDEO_CODEC=av1` (~30–50% smaller than H.264, much slower to encode — use a bigger worker or off-peak).
- `MEDIA_VIDEO_PRESET=slow` (smaller files for more CPU).
- R2 Infrequent Access storage class for old media; lifecycle rules to delete abandoned `pending` uploads.
- Per-owner dedupe via `media.sha256` (column exists; implement reference counting before sharing objects).

## Production setup (Cloudflare R2)
1. Create bucket (EU jurisdiction if you serve EU users), API token with Object Read & Write.
2. Bucket **CORS**: allow `PUT, GET` from your app origin with header `Content-Type` (needed for browser uploads).
3. Env: `S3_ENDPOINT=<account>.r2.cloudflarestorage.com S3_USE_SSL=true S3_REGION=auto S3_ACCESS_KEY/SECRET`.
4. Public media (avatars, product photos): a second public bucket or custom domain → `S3_PUBLIC_BASE_URL`.
5. Add the storage origin to CSP `connect-src`/`img-src` in `infra/Caddyfile` if you tighten it.
Optional malware scanning: add a ClamAV container and scan in `media.process` before marking `ready`.

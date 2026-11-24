# Media: any file type, compressed like the big platforms

| Upload | Stored as | Typical saving |
|---|---|---|
| JPEG/PNG/HEIC/AVIF/WebP photo | WebP (or AVIF with `MEDIA_IMAGE_FORMAT=avif`) at 320/640/1280/1920 px (never upscaled), EXIF/GPS stripped | 70–95 % |
| GIF | kept (animated) – convert to MP4 in your app if needed | – |
| Video (MOV, MP4, WebM, MKV…) | H.264 MP4 CRF 26 `veryfast`, ≤ 720p (`MEDIA_VIDEO_MAX_HEIGHT`), AAC 96 kbps, `+faststart`, WebP poster; AV1 option (`MEDIA_VIDEO_CODEC=av1`) ~30–50 % smaller but slow | 60–95 % |
| Audio | AAC 96 kbps M4A, metadata stripped | 50–90 % vs WAV |
| CSV, JSON, TXT, XML, SVG | gzip at rest (served with `Content-Encoding: gzip`) | 60–90 % |
| PDF, ZIP, DOCX/XLSX/PPTX, 7z, RAR | kept byte-for-byte (already compressed; altering PDFs can break signatures) | – |
| Executables (exe, ELF, Mach-O, scripts, apk) | **rejected** | – |

Plus: per-user de-duplication (same file twice = stored once), originals deleted after processing
(`MEDIA_KEEP_ORIGINAL=false`), immutable cache headers for public files, private files behind access checks
or 15-minute presigned URLs.

## Tools
The Docker image includes **ffmpeg** and **libvips** (`vips` CLI). Without them (e.g. dev on a machine without
ffmpeg) files are stored uncompressed – still safe. `apt install ffmpeg libvips-tools` in WSL for full dev parity.

## Using media in a feature
```go
m, err := mediaSvc.Upload(ctx, userID, fh.Filename, file, "public") // returns immediately, processing async
```
```templ
@components.Picture(m, "Alt text describing the image", "(min-width: 768px) 50vw, 100vw", false)
@components.Video(m)
```
Store `media.id` in your table (`cover_id uuid REFERENCES media (id) ON DELETE SET NULL`) and check ownership
when a user references an upload (see `items.handlers.ownCover`).

## Going to object storage (recommended once media > a few GB)
Set `STORAGE_DRIVER=s3` + `S3_*` (Cloudflare R2: no egress fees). Public files can be served from a custom
domain/CDN via `S3_PUBLIC_URL`. For very large uploads, add a presigned-PUT endpoint using `storage.S3.Presign("PUT", key, ttl)`
so browsers upload straight to the bucket (your server never carries the bytes).

## Costs (rule of thumb)
1,000 users × 50 photos × ~150 KB after compression ≈ 7.5 GB → within R2's free 10 GB.

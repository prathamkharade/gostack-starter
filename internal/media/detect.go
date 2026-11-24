// Package media detects, validates and compresses uploaded files the way large
// platforms do: re-encode images to WebP/AVIF in several widths, transcode
// video to H.264/AV1 MP4 with faststart + poster, audio to AAC, gzip
// compressible text, keep already-compressed formats (zip, pdf, docx…) as-is,
// strip metadata (EXIF GPS = personal data) and reject executables.
//
// The Processor shells out to ffmpeg/ffprobe (and libvips "vips" when
// installed, which is faster and lighter for images). No CGO.
package media

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"
)

type Kind string

const (
	Image    Kind = "image"
	Video    Kind = "video"
	Audio    Kind = "audio"
	PDF      Kind = "pdf"
	Archive  Kind = "archive"
	Document Kind = "document"
	Text     Kind = "text"
	Other    Kind = "other"
	Blocked  Kind = "blocked"
)

// Detect identifies a file from its first bytes (never trust the extension or
// the browser's Content-Type) and the original filename for zip-based formats.
func Detect(head []byte, filename string) (mime string, kind Kind) {
	ext := strings.ToLower(filepath.Ext(filename))

	switch {
	case bytes.HasPrefix(head, []byte("MZ")),
		bytes.HasPrefix(head, []byte{0x7f, 'E', 'L', 'F'}),
		bytes.HasPrefix(head, []byte{0xcf, 0xfa, 0xed, 0xfe}),
		bytes.HasPrefix(head, []byte{0xca, 0xfe, 0xba, 0xbe}),
		bytes.HasPrefix(head, []byte("#!")):
		return "application/octet-stream", Blocked
	}

	// ISO-BMFF family (mp4, mov, heic, avif, m4a) – brand at bytes 8..12
	if len(head) >= 12 && string(head[4:8]) == "ftyp" {
		switch brand := string(head[8:12]); brand {
		case "avif", "avis":
			return "image/avif", Image
		case "heic", "heix", "hevc", "heim", "heis", "mif1", "msf1":
			return "image/heic", Image
		case "M4A ", "M4B ":
			return "audio/mp4", Audio
		case "qt  ":
			return "video/quicktime", Video
		default:
			return "video/mp4", Video
		}
	}
	if bytes.HasPrefix(head, []byte{0x1a, 0x45, 0xdf, 0xa3}) {
		if ext == ".mkv" {
			return "video/x-matroska", Video
		}
		return "video/webm", Video
	}

	mime = http.DetectContentType(head)
	if i := strings.IndexByte(mime, ';'); i > 0 {
		mime = mime[:i]
	}

	switch {
	case mime == "image/svg+xml" || (strings.HasPrefix(mime, "text/") && ext == ".svg"):
		// SVG can contain script: stored as a downloadable document, never inlined.
		return "image/svg+xml", Document
	case strings.HasPrefix(mime, "image/"):
		return mime, Image
	case strings.HasPrefix(mime, "video/"):
		return mime, Video
	case strings.HasPrefix(mime, "audio/") || mime == "application/ogg":
		return mime, Audio
	case mime == "application/pdf":
		return mime, PDF
	case mime == "application/zip":
		switch ext { // OOXML/ODF are zip containers
		case ".docx":
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Document
		case ".xlsx":
			return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Document
		case ".pptx":
			return "application/vnd.openxmlformats-officedocument.presentationml.presentation", Document
		case ".odt", ".ods", ".odp", ".epub":
			return mime, Document
		case ".apk", ".jar", ".ipa", ".xpi":
			return "application/octet-stream", Blocked
		}
		return mime, Archive
	case mime == "application/x-gzip", mime == "application/x-rar-compressed",
		bytes.HasPrefix(head, []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}),
		bytes.HasPrefix(head, []byte{0x28, 0xb5, 0x2f, 0xfd}): // zstd
		if mime == "application/octet-stream" {
			mime = "application/x-archive"
		}
		return mime, Archive
	case mime == "text/html":
		return "text/plain", Text // never serve uploaded HTML as HTML (stored XSS)
	case strings.HasPrefix(mime, "text/"):
		switch ext {
		case ".csv":
			return "text/csv", Text
		case ".json":
			return "application/json", Text
		case ".md":
			return "text/markdown", Text
		}
		return mime, Text
	}
	return "application/octet-stream", Other
}

// Compressible reports whether gzip-at-rest is worthwhile.
func Compressible(mime string, kind Kind) bool {
	return kind == Text || mime == "image/svg+xml" || mime == "application/json" || mime == "application/xml"
}

// Inline reports whether a type is safe to display in the browser. Everything
// else is served with Content-Disposition: attachment.
func Inline(mime string) bool {
	switch mime {
	case "image/webp", "image/avif", "image/jpeg", "image/png", "image/gif",
		"video/mp4", "video/webm", "audio/mp4", "audio/mpeg", "audio/ogg", "application/pdf":
		return true
	}
	return false
}

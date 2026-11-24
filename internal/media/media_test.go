package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		head     []byte
		name     string
		wantMIME string
		wantKind Kind
	}{
		{[]byte("\x89PNG\r\n\x1a\n0000"), "a.png", "image/png", Image},
		{[]byte("\xff\xd8\xff\xe0"), "a.jpg", "image/jpeg", Image},
		{[]byte("\x00\x00\x00\x18ftypheic0000"), "a.heic", "image/heic", Image},
		{[]byte("\x00\x00\x00\x18ftypisom0000"), "a.mp4", "video/mp4", Video},
		{[]byte("%PDF-1.7"), "a.pdf", "application/pdf", PDF},
		{[]byte("PK\x03\x04"), "report.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", Document},
		{[]byte("PK\x03\x04"), "files.zip", "application/zip", Archive},
		{[]byte("MZ\x90\x00"), "invoice.pdf", "application/octet-stream", Blocked}, // exe disguised as pdf
		{[]byte("<html><script>alert(1)</script>"), "x.html", "text/plain", Text},
		{[]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg">`), "a.svg", "image/svg+xml", Document},
		{[]byte("name,email\n"), "a.csv", "text/csv", Text},
	}
	for _, c := range cases {
		mime, kind := Detect(c.head, c.name)
		if mime != c.wantMIME || kind != c.wantKind {
			t.Errorf("%s: got %s/%s want %s/%s", c.name, mime, kind, c.wantMIME, c.wantKind)
		}
	}
}

func TestPassthroughGzipsText(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.csv")
	os.WriteFile(in, []byte(strings.Repeat("id,name,email\n1,Jane,jane@example.com\n", 500)), 0o600)
	p := NewProcessor(Options{})
	res, err := p.Process(context.Background(), in, dir, "text/csv", Text)
	if err != nil {
		t.Fatal(err)
	}
	o := res.Outputs[0]
	if o.Encoding != "gzip" || o.Size >= size(in)/5 {
		t.Fatalf("expected strong gzip compression, got %+v (orig %d)", o, size(in))
	}
}

// Integration: runs only when ffmpeg is installed (it is in the Docker image).
func TestFFmpegPipelines(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	ctx := context.Background()
	img := filepath.Join(dir, "in.png")
	vid := filepath.Join(dir, "in.mov")
	if err := run(ctx, "ffmpeg", "-nostdin", "-y", "-f", "lavfi", "-i", "testsrc=size=1600x900", "-frames:v", "1", img); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, "ffmpeg", "-nostdin", "-y", "-f", "lavfi", "-i", "testsrc=size=1920x1080:rate=30:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "pcm_s16le", vid); err != nil {
		t.Fatal(err)
	}
	p := NewProcessor(Options{ImageWidths: []int{320, 640, 1280, 1920}, VideoMaxHeight: 720})

	ir, err := p.Process(ctx, img, dir, "image/png", Image)
	if err != nil {
		t.Fatal(err)
	}
	if len(ir.Outputs) != 3 { // 1920 > 1600 is skipped: never upscale
		t.Fatalf("want 3 widths, got %d", len(ir.Outputs))
	}
	if ir.Outputs[0].MIME != "image/webp" || ir.Outputs[0].Width != 320 {
		t.Fatalf("bad image output %+v", ir.Outputs[0])
	}

	vr, err := p.Process(ctx, vid, dir, "video/quicktime", Video)
	if err != nil {
		t.Fatal(err)
	}
	v := vr.Outputs[0]
	if v.Height != 720 || v.Size >= size(vid) || vr.Outputs[1].Name != "poster" {
		t.Fatalf("video not transcoded/downscaled: %+v (orig %d bytes)", vr.Outputs, size(vid))
	}
	t.Logf("video %d -> %d bytes", size(vid), v.Size)
}

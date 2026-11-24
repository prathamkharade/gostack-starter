package media

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// Options mirror config.Media* settings.
type Options struct {
	ImageFormat    string // webp | avif
	ImageQuality   int    // 1-100
	ImageWidths    []int  // responsive widths, e.g. 320,640,1280,1920
	VideoCodec     string // h264 | av1
	VideoMaxHeight int    // e.g. 720
	Timeout        time.Duration
}

// Output is one stored rendition.
type Output struct {
	Name     string // "w640", "video", "poster", "audio", "file"
	Path     string // local temp path
	MIME     string
	Encoding string // "gzip" when compressed at rest
	Width    int
	Height   int
	Size     int64
}

// Result of processing one upload.
type Result struct {
	Outputs  []Output
	Width    int
	Height   int
	Duration float64 // seconds
}

type Processor struct {
	Opt     Options
	ffmpeg  string
	ffprobe string
	vips    string
}

// NewProcessor finds the tools on PATH. Missing tools degrade gracefully:
// files are stored as uploaded (still safe, just bigger).
func NewProcessor(o Options) *Processor {
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Minute
	}
	if o.ImageQuality <= 0 || o.ImageQuality > 100 {
		o.ImageQuality = 75
	}
	if len(o.ImageWidths) == 0 {
		o.ImageWidths = []int{320, 640, 1280, 1920}
	}
	if o.VideoMaxHeight <= 0 {
		o.VideoMaxHeight = 720
	}
	p := &Processor{Opt: o}
	p.ffmpeg, _ = exec.LookPath("ffmpeg")
	p.ffprobe, _ = exec.LookPath("ffprobe")
	p.vips, _ = exec.LookPath("vips")
	return p
}

// Tools reports which encoders are available (shown in logs at startup).
func (p *Processor) Tools() map[string]bool {
	return map[string]bool{"ffmpeg": p.ffmpeg != "", "ffprobe": p.ffprobe != "", "vips": p.vips != ""}
}

// Process converts the file at in (already sniffed) into renditions in outDir.
func (p *Processor) Process(ctx context.Context, in, outDir, mime string, kind Kind) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, p.Opt.Timeout)
	defer cancel()
	switch kind {
	case Image:
		if mime == "image/gif" || p.ffmpeg == "" && p.vips == "" {
			break // animated GIFs and tool-less hosts: keep original
		}
		return p.image(ctx, in, outDir)
	case Video:
		if p.ffmpeg != "" {
			return p.video(ctx, in, outDir)
		}
	case Audio:
		if p.ffmpeg != "" {
			return p.audio(ctx, in, outDir)
		}
	}
	return p.passthrough(in, outDir, mime, kind)
}

// --- images ----------------------------------------------------------------------

func (p *Processor) image(ctx context.Context, in, outDir string) (*Result, error) {
	w, h, _ := p.probe(ctx, in)
	res := &Result{Width: w, Height: h}
	ext, mime := ".webp", "image/webp"
	if p.Opt.ImageFormat == "avif" {
		ext, mime = ".avif", "image/avif"
	}
	widths := p.Opt.ImageWidths
	for i, width := range widths {
		if w > 0 && width > w && i > 0 {
			break // never upscale; always produce at least one rendition
		}
		target := width
		if w > 0 && width > w {
			target = w
		}
		out := filepath.Join(outDir, "w"+strconv.Itoa(width)+ext)
		var err error
		if p.vips != "" {
			// thumbnail = shrink-on-load, low memory; strip removes EXIF/GPS
			q := fmt.Sprintf("[Q=%d,strip]", p.Opt.ImageQuality)
			err = run(ctx, p.vips, "thumbnail", in, out+q, strconv.Itoa(target), "--size", "down")
		} else {
			err = run(ctx, p.ffmpeg, "-nostdin", "-y", "-i", in, "-map_metadata", "-1", "-frames:v", "1",
				"-vf", fmt.Sprintf("scale='min(%d,iw)':-2", target),
				"-c:v", "libwebp", "-quality", strconv.Itoa(p.Opt.ImageQuality), out)
			ext, mime = ".webp", "image/webp"
		}
		if err != nil {
			return nil, err
		}
		ow, oh, _ := p.probe(ctx, out)
		res.Outputs = append(res.Outputs, Output{Name: "w" + strconv.Itoa(width), Path: out, MIME: mime, Width: ow, Height: oh, Size: size(out)})
	}
	return res, nil
}

// --- video -----------------------------------------------------------------------

func (p *Processor) video(ctx context.Context, in, outDir string) (*Result, error) {
	w, h, dur := p.probe(ctx, in)
	res := &Result{Width: w, Height: h, Duration: dur}
	out := filepath.Join(outDir, "video.mp4")
	scale := fmt.Sprintf("scale=-2:'min(%d,ih)'", p.Opt.VideoMaxHeight)
	args := []string{"-nostdin", "-y", "-i", in, "-map_metadata", "-1", "-vf", scale}
	if p.Opt.VideoCodec == "av1" {
		// ~30-50% smaller than H.264; slower to encode. Safari needs AV1 hardware.
		args = append(args, "-c:v", "libsvtav1", "-preset", "8", "-crf", "35")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "26", "-profile:v", "high")
	}
	args = append(args, "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "96k", "-ac", "2", "-movflags", "+faststart", out)
	if err := run(ctx, p.ffmpeg, args...); err != nil {
		return nil, err
	}
	ow, oh, _ := p.probe(ctx, out)
	res.Outputs = append(res.Outputs, Output{Name: "video", Path: out, MIME: "video/mp4", Width: ow, Height: oh, Size: size(out)})

	poster := filepath.Join(outDir, "poster.webp")
	seek := "0"
	if dur > 2 {
		seek = "1"
	}
	if err := run(ctx, p.ffmpeg, "-nostdin", "-y", "-ss", seek, "-i", out, "-frames:v", "1", "-c:v", "libwebp", "-quality", "70", poster); err == nil {
		res.Outputs = append(res.Outputs, Output{Name: "poster", Path: poster, MIME: "image/webp", Width: ow, Height: oh, Size: size(poster)})
	}
	return res, nil
}

// --- audio -----------------------------------------------------------------------

func (p *Processor) audio(ctx context.Context, in, outDir string) (*Result, error) {
	_, _, dur := p.probe(ctx, in)
	out := filepath.Join(outDir, "audio.m4a")
	if err := run(ctx, p.ffmpeg, "-nostdin", "-y", "-i", in, "-map_metadata", "-1", "-vn",
		"-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart", out); err != nil {
		return nil, err
	}
	return &Result{Duration: dur, Outputs: []Output{{Name: "audio", Path: out, MIME: "audio/mp4", Size: size(out)}}}, nil
}

// --- everything else ---------------------------------------------------------------

// passthrough keeps the original bytes; gzip-compresses text-like files when
// it saves at least 10%. PDFs/zip/docx/mp3 are already compressed: recompressing
// wastes CPU for ~0% gain (and altering PDFs can break signatures).
func (p *Processor) passthrough(in, outDir, mime string, kind Kind) (*Result, error) {
	out := Output{Name: "file", Path: in, MIME: mime, Size: size(in)}
	if Compressible(mime, kind) {
		gz := filepath.Join(outDir, "file.gz")
		if err := gzipFile(in, gz); err == nil && size(gz) < out.Size*9/10 {
			out.Path, out.Encoding, out.Size = gz, "gzip", size(gz)
		}
	}
	return &Result{Outputs: []Output{out}}, nil
}

func gzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	zw, _ := gzip.NewWriterLevel(out, gzip.BestCompression)
	if _, err = io.Copy(zw, in); err == nil {
		err = zw.Close()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// --- helpers -------------------------------------------------------------------------

func (p *Processor) probe(ctx context.Context, path string) (w, h int, dur float64) {
	if p.ffprobe == "" {
		return 0, 0, 0
	}
	out, err := exec.CommandContext(ctx, p.ffprobe, "-v", "error", "-print_format", "json",
		"-show_entries", "stream=width,height:format=duration", path).Output()
	if err != nil {
		return 0, 0, 0
	}
	var r struct {
		Streams []struct{ Width, Height int } `json:"streams"`
		Format  struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if json.Unmarshal(out, &r) != nil {
		return 0, 0, 0
	}
	for _, s := range r.Streams {
		if s.Width > 0 {
			w, h = s.Width, s.Height
			break
		}
	}
	dur, _ = strconv.ParseFloat(r.Format.Duration, 64)
	return w, h, dur
}

var ErrTool = errors.New("media: tool failed")

func run(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...) // no shell: args are never interpreted
	out, err := cmd.CombinedOutput()
	if err != nil {
		tail := out
		if len(tail) > 600 {
			tail = tail[len(tail)-600:]
		}
		return fmt.Errorf("%w: %s: %v: %s", ErrTool, filepath.Base(bin), err, tail)
	}
	return nil
}

func size(path string) int64 {
	if st, err := os.Stat(path); err == nil {
		return st.Size()
	}
	return 0
}

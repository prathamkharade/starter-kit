package media

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"app/internal/platform/jobs"
	"app/internal/platform/kit"
	"app/internal/platform/storage"
)

// Compression strategy (what large platforms do, tuned for cost):
//
//	image    → WebP (or AVIF) at q≈72, metadata/EXIF/GPS stripped, 3 sizes (320/1280/2048)
//	video    → H.264 (or HEVC/AV1) CRF 28, max 720p, AAC 96k, faststart, metadata stripped + poster
//	audio    → AAC 96k
//	pdf      → Ghostscript /ebook (150 dpi images)
//	archive  → stored as-is (already compressed)
//	other    → gzip -9 with Content-Encoding when it saves ≥10%
//
// We always keep whichever is smaller (original vs processed) unless the
// processed form is needed for compatibility. Missing tools ⇒ original kept.
// Tools run on untrusted input: the worker container is unprivileged and
// ffmpeg is restricted to local files and common demuxers (SSRF/LFI hardening).

var imageWidths = map[string]int{"thumb": 320, "md": 1280, "lg": 2048}

const ffmpegSafe = "-protocol_whitelist file -format_whitelist mov,matroska,webm,avi,flv,mpegts,ogg,wav,mp3,aac,flac,asf"

func ProcessJob(d *kit.Deps) jobs.Handler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p struct {
			ID uuid.UUID `json:"id"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return jobs.Permanent{Err: err}
		}
		err := process(ctx, d, p.ID)
		if err != nil {
			_, _ = d.DB.Exec(ctx, `UPDATE media SET error=$2 WHERE id=$1`, p.ID, truncate(err.Error(), 500))
		}
		return err
	}
}

func process(ctx context.Context, d *kit.Deps, id uuid.UUID) error {
	var ownerKind, origKey, origType, origName string
	if err := d.DB.QueryRow(ctx, `SELECT kind, storage_key, content_type, original_name FROM media WHERE id=$1 AND status='processing'`, id).
		Scan(&ownerKind, &origKey, &origType, &origName); err != nil {
		return jobs.Permanent{Err: fmt.Errorf("media %s not processable: %w", id, err)}
	}
	dir, err := os.MkdirTemp("", "media-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	in := filepath.Join(dir, "in")
	if err := d.Store.Download(ctx, origKey, in); err != nil {
		return err
	}
	st, _ := os.Stat(in)
	origSize := st.Size()
	sum, sniffed, err := hashAndSniff(in)
	if err != nil {
		return err
	}
	kind := ownerKind
	if sniffed != "application/octet-stream" && sniffed != "text/plain; charset=utf-8" {
		kind = KindOf(sniffed, origName) // trust bytes over the client's claim
	}

	prefix := strings.TrimSuffix(origKey, filepath.Base(origKey))
	cfg := d.Cfg
	r := result{primaryKey: origKey, primaryType: origType, stored: origSize, variants: map[string]Variant{}}

	switch kind {
	case "image":
		if have("vips") {
			fmtExt := cfg.MediaImageFormat
			input := in
			if strings.Contains(sniffed, "gif") {
				input = in + "[n=-1]" // keep animation
			}
			var total int64
			for name, w := range imageWidths {
				out := filepath.Join(dir, name+"."+fmtExt)
				opts := fmt.Sprintf("[Q=%d,keep=none]", cfg.MediaImageQuality)
				if err := run(ctx, "vips", "thumbnail", input, out+opts, strconv.Itoa(w), "--size", "down"); err != nil {
					return err
				}
				size, err := upload(ctx, d.Store, prefix+name+"."+fmtExt, out, "image/"+fmtExt)
				if err != nil {
					return err
				}
				r.variants[name] = Variant{Key: prefix + name + "." + fmtExt, Width: w, Bytes: size, Type: "image/" + fmtExt}
				total += size
			}
			r.primaryKey, r.primaryType, r.stored = r.variants["lg"].Key, r.variants["lg"].Type, total
			delete(r.variants, "lg")
			r.dropOriginal = true
		}
	case "video":
		if have("ffmpeg") {
			out := filepath.Join(dir, "video.mp4")
			args := append(strings.Fields(ffmpegSafe), "-y", "-i", in, "-map_metadata", "-1",
				"-vf", fmt.Sprintf("scale=-2:'min(%d,ih)'", cfg.MediaVideoMaxH))
			args = append(args, videoCodecArgs(cfg.MediaVideoCodec, cfg.MediaVideoCRF, cfg.MediaVideoPreset)...)
			args = append(args, "-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart", out)
			if err := run(ctx, "ffmpeg", args...); err != nil {
				return err
			}
			if fi, _ := os.Stat(out); fi.Size() < origSize || origType != "video/mp4" {
				size, err := upload(ctx, d.Store, prefix+"video.mp4", out, "video/mp4")
				if err != nil {
					return err
				}
				r.primaryKey, r.primaryType, r.stored, r.dropOriginal = prefix+"video.mp4", "video/mp4", size, true
			}
			poster := filepath.Join(dir, "poster.webp")
			pargs := append(strings.Fields(ffmpegSafe), "-y", "-ss", "1", "-i", in, "-frames:v", "1", "-vf", "scale=640:-2", "-c:v", "libwebp", "-quality", "70", poster)
			if run(ctx, "ffmpeg", pargs...) == nil {
				if size, err := upload(ctx, d.Store, prefix+"poster.webp", poster, "image/webp"); err == nil {
					r.variants["poster"] = Variant{Key: prefix + "poster.webp", Width: 640, Bytes: size, Type: "image/webp"}
					r.stored += size
				}
			}
		}
	case "audio":
		if have("ffmpeg") {
			out := filepath.Join(dir, "audio.m4a")
			args := append(strings.Fields(ffmpegSafe), "-y", "-i", in, "-vn", "-map_metadata", "-1", "-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart", out)
			if err := run(ctx, "ffmpeg", args...); err != nil {
				return err
			}
			if fi, _ := os.Stat(out); fi.Size() < origSize {
				size, err := upload(ctx, d.Store, prefix+"audio.m4a", out, "audio/mp4")
				if err != nil {
					return err
				}
				r.primaryKey, r.primaryType, r.stored, r.dropOriginal = prefix+"audio.m4a", "audio/mp4", size, true
			}
		}
	case "document", "other":
		switch {
		case strings.Contains(sniffed, "pdf") && have("gs"):
			out := filepath.Join(dir, "doc.pdf")
			if run(ctx, "gs", "-dSAFER", "-dBATCH", "-dNOPAUSE", "-dQUIET", "-sDEVICE=pdfwrite",
				"-dPDFSETTINGS=/ebook", "-dCompatibilityLevel=1.6", "-sOutputFile="+out, in) == nil {
				if fi, _ := os.Stat(out); fi.Size() < origSize*9/10 {
					size, err := upload(ctx, d.Store, prefix+"doc.pdf", out, "application/pdf")
					if err != nil {
						return err
					}
					r.primaryKey, r.primaryType, r.stored, r.dropOriginal = prefix+"doc.pdf", "application/pdf", size, true
				}
			}
		case compressible(sniffed, origName):
			out := filepath.Join(dir, "gz")
			if err := gzipFile(in, out); err != nil {
				return err
			}
			if fi, _ := os.Stat(out); fi.Size() < origSize*9/10 {
				key := prefix + "file.gz"
				size, err := d.Store.UploadFile(ctx, key, out, storage.PutOpts{ContentType: origType, ContentEncoding: "gzip"})
				if err != nil {
					return err
				}
				r.primaryKey, r.stored, r.encoding, r.dropOriginal = key, size, "gzip", true
			}
		}
	}

	if r.dropOriginal && !cfg.MediaKeepOriginal && r.primaryKey != origKey {
		if err := d.Store.Delete(ctx, origKey); err != nil {
			return err
		}
	} else if r.primaryKey != origKey {
		r.variants["original"] = Variant{Key: origKey, Bytes: origSize, Type: origType}
		r.stored += origSize
	}

	vj, _ := json.Marshal(r.variants)
	var enc any
	if r.encoding != "" {
		enc = r.encoding
	}
	_, err = d.DB.Exec(ctx, `UPDATE media SET status='ready', kind=$2, storage_key=$3, content_type=$4,
		stored_bytes=$5, sha256=$6, variants=$7, content_encoding=$8, error=NULL WHERE id=$1`,
		id, kind, r.primaryKey, r.primaryType, r.stored, sum, vj, enc)
	if err == nil {
		d.Log.Info("media processed", "id", id, "kind", kind, "original_bytes", origSize, "stored_bytes", r.stored)
	}
	return err
}

type result struct {
	primaryKey, primaryType, encoding string
	stored                            int64
	variants                          map[string]Variant
	dropOriginal                      bool
}

func videoCodecArgs(codec string, crf int, preset string) []string {
	switch codec {
	case "av1": // best compression, slowest; widely decodable since ~2023 browsers/phones
		return []string{"-c:v", "libsvtav1", "-crf", strconv.Itoa(crf + 6), "-preset", "8", "-pix_fmt", "yuv420p"}
	case "hevc":
		return []string{"-c:v", "libx265", "-crf", strconv.Itoa(crf), "-preset", preset, "-tag:v", "hvc1", "-pix_fmt", "yuv420p"}
	default: // h264: plays everywhere
		return []string{"-c:v", "libx264", "-crf", strconv.Itoa(crf), "-preset", preset, "-profile:v", "high", "-pix_fmt", "yuv420p"}
	}
}

func have(tool string) bool { _, err := exec.LookPath(tool); return err == nil }

func run(ctx context.Context, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", name, err, truncate(string(out), 300))
	}
	return nil
}

func upload(ctx context.Context, s *storage.Store, key, path, ct string) (int64, error) {
	return s.UploadFile(ctx, key, path, storage.PutOpts{ContentType: ct, CacheControl: "private, max-age=31536000, immutable"})
}

func hashAndSniff(path string) ([]byte, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, "", err
	}
	return h.Sum(nil), http.DetectContentType(head[:n]), nil
}

func compressible(sniffed, name string) bool {
	if strings.HasPrefix(sniffed, "text/") || strings.Contains(sniffed, "json") || strings.Contains(sniffed, "xml") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	return in(ext, ".csv", ".json", ".txt", ".md", ".log", ".svg", ".xml", ".sql", ".rtf", ".doc", ".xls", ".ppt", ".bmp", ".wav")
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
	defer out.Close()
	zw, _ := gzip.NewWriterLevel(out, gzip.BestCompression)
	if _, err := io.Copy(zw, in); err != nil {
		return err
	}
	return zw.Close()
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

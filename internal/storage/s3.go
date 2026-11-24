package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// S3 is a minimal S3-compatible client (PUT/GET/DELETE + presigned URLs).
//
//	Cloudflare R2: Endpoint https://<account>.r2.cloudflarestorage.com, Region "auto"
//	Backblaze B2:  Endpoint https://s3.<region>.backblazeb2.com
//	Hetzner:       Endpoint https://<location>.your-objectstorage.com
//	MinIO (dev):   Endpoint http://localhost:9000, PathStyle true
type S3 struct {
	Endpoint  string // scheme://host[:port], no trailing slash
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	PublicURL string // optional public/CDN base for public objects
	PathStyle bool   // true: endpoint/bucket/key, false: bucket.endpoint/key
	Client    *http.Client
	now       func() time.Time
}

const unsignedPayload = "UNSIGNED-PAYLOAD"

func (s *S3) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now().UTC()
}

func (s *S3) http() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 10 * time.Minute}
}

// objectURL builds the URL for key (path segments escaped per SigV4 rules).
func (s *S3) objectURL(key string) (*url.URL, error) {
	u, err := url.Parse(s.Endpoint)
	if err != nil {
		return nil, err
	}
	if s.PathStyle {
		u.Path = "/" + s.Bucket + "/" + key
	} else {
		u.Host = s.Bucket + "." + u.Host
		u.Path = "/" + key
	}
	u.RawPath = awsEscapePath(u.Path)
	return u, nil
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64, m Meta) error {
	if !ValidKey(key) {
		return fmt.Errorf("storage: invalid key %q", key)
	}
	u, err := s.objectURL(key)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), r)
	if err != nil {
		return err
	}
	req.ContentLength = size
	for k, v := range map[string]string{"Content-Type": m.ContentType, "Content-Encoding": m.ContentEncoding,
		"Cache-Control": m.CacheControl, "Content-Disposition": m.ContentDisposition} {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	return s.do(req, http.StatusOK)
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	u, err := s.objectURL(key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Encoding", "identity") // return stored bytes as-is (no transparent gunzip)
	s.sign(req, unsignedPayload)
	res, err := s.http().Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusNotFound {
		res.Body.Close()
		return nil, ErrNotFound
	}
	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, fmt.Errorf("storage: s3 GET %d: %s", res.StatusCode, b)
	}
	return res.Body, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	u, err := s.objectURL(key)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), nil)
	if err != nil {
		return err
	}
	return s.do(req, http.StatusNoContent, http.StatusOK, http.StatusNotFound)
}

func (s *S3) URL(_ context.Context, key string, public bool, ttl time.Duration) (string, error) {
	if public && s.PublicURL != "" {
		return s.PublicURL + "/" + awsEscapePath(key), nil
	}
	return s.Presign(http.MethodGet, key, ttl)
}

// Presign returns a query-signed URL (GET for downloads, PUT for direct
// browser uploads that bypass your server's bandwidth).
func (s *S3) Presign(method, key string, ttl time.Duration) (string, error) {
	u, err := s.objectURL(key)
	if err != nil {
		return "", err
	}
	if ttl <= 0 || ttl > 7*24*time.Hour {
		ttl = 15 * time.Minute
	}
	t := s.clock()
	scope := s.scope(t)
	q := url.Values{}
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", s.AccessKey+"/"+scope)
	q.Set("X-Amz-Date", t.Format("20060102T150405Z"))
	q.Set("X-Amz-Expires", strconv.Itoa(int(ttl.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")
	canonicalQuery := awsEncodeQuery(q)
	creq := strings.Join([]string{method, u.RawPath, canonicalQuery, "host:" + u.Host + "\n", "host", unsignedPayload}, "\n")
	sig := s.signature(t, creq)
	u.RawQuery = canonicalQuery + "&X-Amz-Signature=" + sig
	return u.String(), nil
}

func (s *S3) do(req *http.Request, ok ...int) error {
	s.sign(req, unsignedPayload)
	res, err := s.http().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	for _, c := range ok {
		if res.StatusCode == c {
			_, _ = io.Copy(io.Discard, res.Body)
			return nil
		}
	}
	b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	return fmt.Errorf("storage: s3 %s %d: %s", req.Method, res.StatusCode, b)
}

// sign adds SigV4 Authorization headers. All headers present on the request
// (plus host) are signed.
func (s *S3) sign(req *http.Request, payloadHash string) {
	t := s.clock()
	req.Header.Set("X-Amz-Date", t.Format("20060102T150405Z"))
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	names := []string{"host"}
	vals := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		names = append(names, lk)
		vals[lk] = strings.Join(v, ",")
	}
	sort.Strings(names)
	var ch strings.Builder
	for _, n := range names {
		ch.WriteString(n + ":" + strings.TrimSpace(vals[n]) + "\n")
	}
	signed := strings.Join(names, ";")
	path := req.URL.RawPath
	if path == "" {
		path = awsEscapePath(req.URL.Path)
	}
	creq := strings.Join([]string{req.Method, path, awsEncodeQuery(req.URL.Query()), ch.String(), signed, payloadHash}, "\n")
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.AccessKey, s.scope(t), signed, s.signature(t, creq)))
}

func (s *S3) scope(t time.Time) string {
	return t.Format("20060102") + "/" + s.Region + "/s3/aws4_request"
}

func (s *S3) signature(t time.Time, canonicalRequest string) string {
	h := sha256.Sum256([]byte(canonicalRequest))
	sts := "AWS4-HMAC-SHA256\n" + t.Format("20060102T150405Z") + "\n" + s.scope(t) + "\n" + hex.EncodeToString(h[:])
	k := hmacSHA256([]byte("AWS4"+s.SecretKey), t.Format("20060102"))
	k = hmacSHA256(k, s.Region)
	k = hmacSHA256(k, "s3")
	k = hmacSHA256(k, "aws4_request")
	return hex.EncodeToString(hmacSHA256(k, sts))
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

// awsEscapePath percent-encodes everything except unreserved chars and '/'.
func awsEscapePath(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '/' || isUnreserved(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func awsEncodeQuery(v url.Values) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vs := append([]string(nil), v[k]...)
		sort.Strings(vs)
		for _, val := range vs {
			parts = append(parts, awsEscape(k)+"="+awsEscape(val))
		}
	}
	return strings.Join(parts, "&")
}

func awsEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if isUnreserved(s[i]) {
			b.WriteByte(s[i])
		} else {
			fmt.Fprintf(&b, "%%%02X", s[i])
		}
	}
	return b.String()
}

func isUnreserved(c byte) bool {
	return 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~'
}

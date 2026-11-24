package storage

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Test vectors from the AWS S3 SigV4 documentation ("examplebucket").
func awsExample() *S3 {
	return &S3{
		Endpoint: "https://s3.amazonaws.com", Region: "us-east-1", Bucket: "examplebucket",
		AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		now: func() time.Time { return time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC) },
	}
}

func TestPresignMatchesAWSVector(t *testing.T) {
	u, err := awsExample().Presign(http.MethodGet, "test.txt", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(u, "X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404") {
		t.Fatalf("signature mismatch: %s", u)
	}
	if !strings.HasPrefix(u, "https://examplebucket.s3.amazonaws.com/test.txt?") {
		t.Fatalf("url: %s", u)
	}
}

func TestHeaderSigningMatchesAWSVector(t *testing.T) {
	s := awsExample()
	req, _ := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	req.Header.Set("Range", "bytes=0-9")
	s.sign(req, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	want := "Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); !strings.HasSuffix(got, want) {
		t.Fatalf("got %s", got)
	}
}

func TestValidKey(t *testing.T) {
	for _, k := range []string{"m/2026/11/abc/w640.webp", "a.txt"} {
		if !ValidKey(k) {
			t.Errorf("%q should be valid", k)
		}
	}
	for _, k := range []string{"", "../etc/passwd", "a/../../b", "/abs", "UPPER", "a b"} {
		if ValidKey(k) {
			t.Errorf("%q should be invalid", k)
		}
	}
}

func TestLocalRoundTrip(t *testing.T) {
	l, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := l.Put(ctx, "m/x/file.txt", strings.NewReader("hello"), 5, Meta{}); err != nil {
		t.Fatal(err)
	}
	rc, err := l.Get(ctx, "m/x/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "hello" {
		t.Fatal("content mismatch")
	}
	if err := l.Delete(ctx, "m/x/file.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Get(ctx, "m/x/file.txt"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := l.Put(ctx, "../escape", strings.NewReader("x"), 1, Meta{}); err == nil {
		t.Fatal("traversal must be rejected")
	}
}

// Package static embeds CSS/JS/images into the binary (one file to deploy) and
// fingerprints them so browsers can cache them forever:
//
//	static.Path("css/app.css") -> "/static/css/app.css?v=3f2a91c0"
package static

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"sync"
)

//go:embed css js vendor img
var FS embed.FS

var (
	once   sync.Once
	hashes map[string]string
)

func load() {
	hashes = map[string]string{}
	_ = fs.WalkDir(FS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := FS.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		hashes[p] = hex.EncodeToString(sum[:4])
		return nil
	})
}

// Path returns the cache-busted URL of an embedded asset.
func Path(name string) string {
	once.Do(load)
	if h, ok := hashes[name]; ok {
		return "/static/" + name + "?v=" + h
	}
	return "/static/" + name
}

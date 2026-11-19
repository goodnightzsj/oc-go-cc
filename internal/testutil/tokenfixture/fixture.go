// Package tokenfixture primes the real tokenizer for tests without network access.
package tokenfixture

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/pkoukk/tiktoken-go"
)

//go:embed cl100k_base.tiktoken.gz
var vocabulary []byte

// Prime runs before m.Run; later counters use tiktoken's in-process encoding cache.
func Prime() (err error) {
	r, err := gzip.NewReader(bytes.NewReader(vocabulary))
	if err != nil {
		return err
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != "223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7" {
		return errors.New("cl100k_base fixture checksum mismatch")
	}
	dir, err := os.MkdirTemp("", "routatic-token-fixture-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	// tiktoken-go v0.1.8 names cache files by the SHA-1 of the source URL.
	key := sha1.Sum([]byte("https://openaipublic.blob.core.windows.net/encodings/cl100k_base.tiktoken"))
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%x", key)), data, 0600); err != nil {
		return err
	}
	previous, set := os.LookupEnv("TIKTOKEN_CACHE_DIR")
	if err := os.Setenv("TIKTOKEN_CACHE_DIR", dir); err != nil {
		return err
	}
	defer func() {
		if set {
			err = errors.Join(err, os.Setenv("TIKTOKEN_CACHE_DIR", previous))
		} else {
			err = errors.Join(err, os.Unsetenv("TIKTOKEN_CACHE_DIR"))
		}
	}()
	_, err = tiktoken.GetEncoding("cl100k_base")
	return err
}

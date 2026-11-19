package tokenfixture

import (
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/pkoukk/tiktoken-go"
)

type rejectNetwork struct{ calls int }

func (r *rejectNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls++
	return nil, errors.New("tokenizer must not access the network")
}

func TestPrimeUsesRealVocabularyWithoutNetwork(t *testing.T) {
	previous := http.DefaultClient
	blocked := &rejectNetwork{}
	http.DefaultClient = &http.Client{Transport: blocked}
	t.Cleanup(func() { http.DefaultClient = previous })
	t.Setenv("TIKTOKEN_CACHE_DIR", t.TempDir())
	if err := Prime(); err != nil {
		t.Fatal(err)
	}
	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		t.Fatal(err)
	}
	if got := enc.Encode("hello world", nil, nil); !slices.Equal(got, []int{15339, 1917}) {
		t.Fatalf("real token IDs changed: %v", got)
	}
	if blocked.calls != 0 {
		t.Fatalf("network calls=%d", blocked.calls)
	}
}

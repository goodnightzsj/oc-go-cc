package update

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchChecksumMatchesAssetName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("aaaa  other-file\nbbbb  *routatic-proxy_linux-amd64\n"))
	}))
	defer srv.Close()

	got, err := fetchChecksum(srv.URL, "routatic-proxy_linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != "bbbb" {
		t.Fatalf("checksum = %q, want bbbb", got)
	}
	if _, err := fetchChecksum(srv.URL, "missing"); err == nil {
		t.Fatal("expected a missing entry to be an error")
	}
}

func TestGetChecksumsURLRequiresAsset(t *testing.T) {
	release := &GitHubRelease{TagName: "v1.2.3"}
	if _, err := GetChecksumsURL(release); err == nil {
		t.Fatal("a release without checksums.txt must be refused")
	}
	release.Assets = append(release.Assets, struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{Name: "checksums.txt", BrowserDownloadURL: "https://example.test/checksums.txt"})
	if got, err := GetChecksumsURL(release); err != nil || got != "https://example.test/checksums.txt" {
		t.Fatalf("GetChecksumsURL = %q, %v", got, err)
	}
}

package update

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// GitHubRelease represents a GitHub release
type GitHubRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// GetLatestRelease fetches the latest release for the specified channel ("stable" or "beta")
func GetLatestRelease(channel string) (*GitHubRelease, error) {
	var url string

	if channel == "beta" {
		// For beta, get all releases and find the latest prerelease with -beta- in tag
		url = "https://api.github.com/repos/routatic/proxy/releases?per_page=20"
	} else {
		// For stable, use the /latest endpoint
		url = "https://api.github.com/repos/routatic/proxy/releases/latest"
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	if channel == "beta" {
		// Parse as array of releases
		var releases []GitHubRelease
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return nil, fmt.Errorf("failed to parse releases: %w", err)
		}

		// Find the latest beta release. Real tags are v0.6.6-beta.19, so the
		// separator is a dot; matching "-beta-" found nothing and left the beta
		// channel permanently dark. Any prerelease counts as a beta.
		for _, release := range releases {
			if release.Prerelease && semver.Prerelease(release.TagName) != "" {
				return &release, nil
			}
		}
		return nil, fmt.Errorf("no beta releases found")
	}

	// Parse as single release
	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}

	return &release, nil
}

// GetAssetURL finds the download URL for the current platform
func GetAssetURL(release *GitHubRelease) (string, string, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Map Go architecture to asset naming
	archMap := map[string]string{
		"amd64": "amd64",
		"arm64": "arm64",
	}

	assetArch, ok := archMap[goarch]
	if !ok {
		return "", "", fmt.Errorf("unsupported architecture: %s", goarch)
	}

	// Build expected asset name
	// Format: routatic-proxy_{os}-{arch} or routatic-proxy_{os}-{arch}.exe for Windows
	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}

	expectedName := fmt.Sprintf("routatic-proxy_%s-%s%s", goos, assetArch, ext)

	for _, asset := range release.Assets {
		if asset.Name == expectedName {
			return asset.BrowserDownloadURL, expectedName, nil
		}
	}

	return "", "", fmt.Errorf("no matching asset found for %s-%s", goos, goarch)
}

// GetChecksumsURL returns the release's checksums asset. It is required: an
// installer that downloads and runs a binary without verifying it trusts
// whatever the network and the release account happen to serve.
func GetChecksumsURL(release *GitHubRelease) (string, error) {
	for _, asset := range release.Assets {
		if asset.Name == "checksums.txt" {
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("release %s does not publish checksums.txt; refusing an unverifiable update", release.TagName)
}

// fetchChecksum reads checksums.txt and returns the SHA-256 recorded for
// filename.
func fetchChecksum(checksumURL, filename string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(checksumURL)
	if err != nil {
		return "", fmt.Errorf("failed to download checksums: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksums download failed with status %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == filename {
			return strings.ToLower(fields[0]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read checksums: %w", err)
	}
	return "", fmt.Errorf("checksums.txt has no entry for %s", filename)
}

// DownloadAndInstall downloads the binary, verifies it against the release's
// checksums, and replaces the current executable. The temp file is created
// beside the executable: a rename across filesystems fails with EXDEV, which
// made the update impossible whenever TMPDIR lived on another mount.
func DownloadAndInstall(url, filename, checksumURL string) error {
	want, err := fetchChecksum(checksumURL, filename)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Get current executable path first: the temp file belongs in its
	// directory so the final rename stays on one filesystem.
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(execPath), ".routatic-proxy-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file next to the executable: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmpFile, hasher), resp.Body); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if got := hex.EncodeToString(hasher.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", filename, got, want)
	}

	// Make executable on Unix
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			return fmt.Errorf("failed to make executable: %w", err)
		}
	}

	// Keep the old binary until the new one is in place, so a failed install
	// restores it instead of leaving the installation without a working
	// executable.
	oldPath := execPath + ".old"
	_ = os.Remove(oldPath)
	if err := os.Rename(execPath, oldPath); err != nil {
		return fmt.Errorf("failed to move the current executable aside: %w", err)
	}
	if err := os.Rename(tmpPath, execPath); err != nil {
		_ = os.Rename(oldPath, execPath)
		return fmt.Errorf("failed to install new executable: %w", err)
	}
	_ = os.Remove(oldPath)

	return nil
}

// IsNewerVersion reports whether candidate is a newer version than current.
// Uses semantic version comparison via golang.org/x/mod/semver.
func IsNewerVersion(current, candidate string) bool {
	if semver.IsValid(candidate) && semver.IsValid(current) {
		return semver.Compare(candidate, current) > 0
	}
	// Fallback for dev versions or non-semver strings
	return candidate != current && candidate > current
}

// CheckForUpdate checks if a newer version is available
func CheckForUpdate(currentVersion string, channel string) (*GitHubRelease, error) {
	release, err := GetLatestRelease(channel)
	if err != nil {
		return nil, err
	}

	// Compare versions using semantic versioning
	if IsNewerVersion(currentVersion, release.TagName) {
		return release, nil
	}

	return nil, nil // No update available
}

package gui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/debug"
)

type channelSet struct {
	Actual    []string `json:"actual"`
	Available []string `json:"available"`
}

type channelData map[string]channelSet

// Only this metadata projection is persisted. Capture data and request IDs
// never enter the catalogue or its API response.
type channelSnapshot struct {
	DefaultTarget  string      `json:"default_target"`
	CaptureEnabled bool        `json:"capture_enabled"`
	Channels       channelData `json:"channels"`
	LastChecked    *time.Time  `json:"last_checked,omitempty"`
	Error          string      `json:"error,omitempty"`
}

type capturePosition struct {
	info   os.FileInfo
	offset int64
}

type channelCatalog struct {
	scanMu    sync.Mutex // One scan owns file offsets.
	mu        sync.RWMutex
	directory string
	data      channelData // Immutable after publication; readers never wait for disk I/O.
	checked   *time.Time
	err       string
	positions map[string]capturePosition
}

var defaultChannels = func() channelData {
	raw, err := assets.ReadFile("assets/cline-pass-channels.json")
	if err != nil {
		panic(err)
	}
	data, err := decodeChannelData(raw)
	if err != nil {
		panic(err)
	}
	return data
}()

var channelModelPattern = regexp.MustCompile(`^[A-Za-z0-9_./:-]{1,200}$`)

func captureDirectory(cfg *config.DebugCapture) string {
	if cfg == nil || !cfg.Enabled {
		return ""
	}
	if cfg.Directory == "" {
		// Same documented default as debug.Storage.ensureDirectory. Discovery
		// is read-only with respect to capture settings and capture files.
		return config.ExpandHome("~/.config/routatic-proxy/debug")
	}
	return config.ExpandHome(cfg.Directory)
}

func (s *Server) channelDirectory() string {
	if s.atomicCfg == nil {
		return ""
	}
	return captureDirectory(s.atomicCfg.Get().Logging.DebugCapture)
}

func (c *channelCatalog) snapshot(directory string) channelSnapshot {
	result := channelSnapshot{DefaultTarget: config.DefaultClinePassChannel, CaptureEnabled: directory != "", Channels: defaultChannels}
	if directory == "" {
		return result // Disabled means the built-in JSON, never a previous user's cache.
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.directory == directory {
		if c.data != nil {
			result.Channels = c.data
		}
		result.LastChecked, result.Error = c.checked, c.err
	}
	return result
}

func (s *Server) handleChannelCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, s.channels.snapshot(s.channelDirectory()))
}

func (s *Server) channelLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if directory := s.channelDirectory(); directory != "" {
			_ = s.channels.refresh(ctx, directory) // Safe failure status is exposed by the API.
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *channelCatalog) refresh(ctx context.Context, directory string) (err error) {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	if c.directory != directory {
		c.positions = nil
		c.mu.Lock()
		c.directory, c.data, c.checked, c.err = directory, nil, nil, ""
		c.mu.Unlock()
	}
	defer func() {
		if ctx.Err() != nil {
			return
		}
		checked := time.Now().UTC()
		c.mu.Lock()
		defer c.mu.Unlock()
		c.directory, c.checked, c.err = directory, &checked, ""
		if err != nil {
			c.err = "capture_read_failed"
		}
	}()
	current := c.snapshot(directory).Channels
	cachePath := filepath.Join(directory, "channel-catalog.json")
	if c.positions == nil {
		raw, readErr := readChannelCache(cachePath)
		if readErr == nil {
			cached, decodeErr := decodeChannelData(raw)
			if decodeErr != nil {
				return decodeErr
			}
			current = mergeChannels(current, cached)
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return readErr
		}
		c.mu.Lock()
		c.data = current
		c.mu.Unlock()
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	positions := make(map[string]capturePosition)
	found := make(channelData)
	for _, entry := range entries { // ReadDir is sorted; rotation filenames are chronological.
		if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), "capture-") || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		position, scanErr := scanChannelFile(ctx, filepath.Join(directory, entry.Name()), c.positions[entry.Name()], found)
		if scanErr != nil {
			return scanErr
		}
		positions[entry.Name()] = position
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	next := mergeChannels(current, found)
	before, _ := json.Marshal(current)
	after, _ := json.MarshalIndent(next, "", "  ")
	canonical, _ := json.Marshal(next)
	if !bytes.Equal(before, canonical) {
		if err := writeConfigFile(cachePath, append(after, '\n')); err != nil {
			return err
		}
	}
	c.positions = positions
	c.mu.Lock()
	c.data = next
	c.mu.Unlock()
	return nil
}

func readChannelCache(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, (1<<20)+1))
}

func scanChannelFile(ctx context.Context, path string, previous capturePosition, found channelData) (capturePosition, error) {
	file, err := os.Open(path)
	if err != nil {
		return previous, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return previous, err
	}
	position := capturePosition{info: info}
	if previous.info != nil && os.SameFile(info, previous.info) && info.Size() >= previous.offset {
		position.offset = previous.offset
	}
	reader := bufio.NewReader(io.NewSectionReader(file, position.offset, info.Size()-position.offset))
	for {
		if err := ctx.Err(); err != nil {
			return previous, err
		}
		line, err := readCaptureLine(reader)
		if errors.Is(err, io.EOF) {
			return position, nil // A writer's unfinished last line is retried next hour.
		}
		if err != nil {
			return previous, err
		}
		var entry debug.CaptureEntry
		if json.Unmarshal(line, &entry) != nil {
			return previous, errors.New("invalid capture JSON")
		}
		if entry.Provider == "cline-pass" && entry.Phase == debug.PhaseUpstreamResponse {
			if err := readChannelResponse(entry.Data, found); err != nil {
				return previous, err
			}
		}
		position.offset += int64(len(line))
	}
}

func readCaptureLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > 64<<20 {
			return nil, errors.New("capture entry exceeds 64 MiB")
		}
		line = append(line, part...)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return line, err
		}
	}
}

type capturedRouting struct {
	ProviderMetadata struct {
		Gateway struct {
			Routing struct {
				CanonicalSlug      string   `json:"canonicalSlug"`
				FinalProvider      string   `json:"finalProvider"`
				FallbacksAvailable []string `json:"fallbacksAvailable"`
			} `json:"routing"`
		} `json:"gateway"`
	} `json:"provider_metadata"`
}

func readChannelResponse(body string, found channelData) error {
	var values channelSet
	var model, canonical string
	ambiguous := false
	frame := func(data []byte) {
		var chunk struct {
			capturedRouting
			Model   string `json:"model"`
			Choices []struct {
				Delta   capturedRouting `json:"delta"`
				Message capturedRouting `json:"message"`
			} `json:"choices"`
		}
		if json.Unmarshal(data, &chunk) != nil {
			return
		}
		if channelModelPattern.MatchString(chunk.Model) {
			if model != "" && model != chunk.Model {
				ambiguous = true
			}
			model = chunk.Model
		}
		remember := func(metadata capturedRouting) {
			routing := metadata.ProviderMetadata.Gateway.Routing
			if channelModelPattern.MatchString(routing.CanonicalSlug) {
				if canonical != "" && canonical != routing.CanonicalSlug {
					ambiguous = true
				}
				canonical = routing.CanonicalSlug
			}
			values.Actual = append(values.Actual, routing.FinalProvider)
			values.Available = append(values.Available, routing.FallbacksAvailable...)
		}
		remember(chunk.capturedRouting)
		for _, choice := range chunk.Choices {
			remember(choice.Message)
			remember(choice.Delta)
		}
	}
	complete := false
	if json.Valid([]byte(body)) {
		frame([]byte(body))
		complete = true
	} else {
		scanner := bufio.NewScanner(strings.NewReader(body))
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			if data, ok := strings.CutPrefix(scanner.Text(), "data:"); ok {
				if strings.TrimSpace(data) == "[DONE]" {
					complete = true
					break
				}
				frame([]byte(data))
			}
		}
		if scanner.Err() != nil {
			return errors.New("capture SSE frame exceeds limit")
		}
	}
	if model == "" {
		model = canonical
	}
	if complete && !ambiguous && model != "" {
		current := found[model]
		current.Actual = append(current.Actual, values.Actual...)
		current.Available = append(current.Available, values.Available...)
		found[model] = current
	}
	return nil
}

func mergeChannels(left, right channelData) channelData {
	out := make(channelData)
	for _, data := range []channelData{left, right} {
		for model, values := range data {
			if !channelModelPattern.MatchString(model) {
				continue
			}
			current := out[model]
			current.Actual = append(current.Actual, values.Actual...)
			current.Available = append(current.Available, values.Available...)
			out[model] = current
		}
	}
	for model, values := range out {
		clean := func(names []string) []string {
			result := make([]string, 0, len(names))
			for _, name := range names {
				if name == "zai" {
					name = "z-ai"
				}
				if len(name) <= 128 && config.ValidChannelSlug(name) {
					result = append(result, name)
				}
			}
			slices.Sort(result)
			return slices.Compact(result)
		}
		values.Actual, values.Available = clean(values.Actual), clean(values.Available)
		values.Available = slices.DeleteFunc(values.Available, func(name string) bool { return slices.Contains(values.Actual, name) })
		if len(values.Actual)+len(values.Available) == 0 {
			delete(out, model)
		} else {
			out[model] = values
		}
	}
	return out
}

func decodeChannelData(raw []byte) (channelData, error) {
	var data channelData
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(raw) > 1<<20 || decoder.Decode(&data) != nil || data == nil {
		return nil, errors.New("invalid channel catalogue")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid channel catalogue suffix")
	}
	return mergeChannels(nil, data), nil
}

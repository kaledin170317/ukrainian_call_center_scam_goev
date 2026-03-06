package http

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type PreparedCDRMeta struct {
	ID              string
	Path            string
	OriginalName    string
	NormalizedBytes int64
	RowsCount       int64
	CreatedAt       time.Time
}

type PreparedCDRStore struct {
	mu      sync.RWMutex
	dir     string
	ttl     time.Duration
	entries map[string]PreparedCDRMeta
}

func NewPreparedCDRStore(dir string, ttl time.Duration) (*PreparedCDRStore, error) {
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "billing-cdr-*")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir temp dir: %w", err)
	}

	return &PreparedCDRStore{
		dir:     dir,
		ttl:     ttl,
		entries: make(map[string]PreparedCDRMeta, 32),
	}, nil
}

func (s *PreparedCDRStore) SaveNormalized(src io.Reader, originalName string) (PreparedCDRMeta, error) {
	now := time.Now()
	s.cleanup(now)

	id, err := randomID()
	if err != nil {
		return PreparedCDRMeta{}, err
	}

	path := filepath.Join(s.dir, id+".cdr")
	f, err := os.Create(path)
	if err != nil {
		return PreparedCDRMeta{}, fmt.Errorf("create normalized file: %w", err)
	}

	var (
		bytesWritten int64
		rowsCount    int64
		writeErr     error
	)

	defer func() {
		_ = f.Close()
		if writeErr != nil {
			_ = os.Remove(path)
		}
	}()

	sc := bufio.NewScanner(src)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	firstLine := true
	bw := bufio.NewWriterSize(f, 256*1024)

	for sc.Scan() {
		line := sc.Text()
		if firstLine {
			line = strings.TrimPrefix(line, "\uFEFF")
			firstLine = false
		}

		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}

		n, err := bw.WriteString(line)
		if err != nil {
			writeErr = fmt.Errorf("write normalized line: %w", err)
			return PreparedCDRMeta{}, writeErr
		}
		bytesWritten += int64(n)

		n, err = bw.WriteString("\n")
		if err != nil {
			writeErr = fmt.Errorf("write normalized newline: %w", err)
			return PreparedCDRMeta{}, writeErr
		}
		bytesWritten += int64(n)
		rowsCount++
	}

	if err := sc.Err(); err != nil {
		writeErr = fmt.Errorf("read source file: %w", err)
		return PreparedCDRMeta{}, writeErr
	}

	if err := bw.Flush(); err != nil {
		writeErr = fmt.Errorf("flush normalized file: %w", err)
		return PreparedCDRMeta{}, writeErr
	}

	if err := f.Close(); err != nil {
		writeErr = fmt.Errorf("close normalized file: %w", err)
		return PreparedCDRMeta{}, writeErr
	}

	meta := PreparedCDRMeta{
		ID:              id,
		Path:            path,
		OriginalName:    originalName,
		NormalizedBytes: bytesWritten,
		RowsCount:       rowsCount,
		CreatedAt:       now,
	}

	s.mu.Lock()
	s.entries[id] = meta
	s.mu.Unlock()

	return meta, nil
}

func (s *PreparedCDRStore) Get(id string) (PreparedCDRMeta, bool) {
	s.mu.RLock()
	meta, ok := s.entries[id]
	s.mu.RUnlock()
	if !ok {
		return PreparedCDRMeta{}, false
	}

	if time.Since(meta.CreatedAt) > s.ttl {
		s.delete(id, meta.Path)
		return PreparedCDRMeta{}, false
	}

	return meta, true
}

func (s *PreparedCDRStore) cleanup(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, meta := range s.entries {
		if now.Sub(meta.CreatedAt) <= s.ttl {
			continue
		}
		_ = os.Remove(meta.Path)
		delete(s.entries, id)
	}
}

func (s *PreparedCDRStore) delete(id, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.Remove(path)
	delete(s.entries, id)
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}

	return hex.EncodeToString(buf), nil
}

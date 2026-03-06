// Copyright (c) 2023-2026, KNS Group LLC ("YADRO").
// All Rights Reserved.
// This software contains the intellectual property of YADRO
// or is licensed to YADRO from third parties. Use of this
// software and the intellectual property contained therein is expressly
// limited to the terms and conditions of the License Agreement under which
// it is provided by YADRO.

package http

import (
	"io"
	"sync"
	"time"
)

// CDRProgressResponse is polled by UI while POST /api/v1/cdr/tariff is in-flight.
// progress_pct is null when total_bytes is unknown.
type CDRProgressResponse struct {
	Status      string `json:"status"` // processing | done | error
	ProgressPct *int   `json:"progress_pct"`
	ReadBytes   int64  `json:"read_bytes"`
	TotalBytes  int64  `json:"total_bytes"`
	UpdatedAt   string `json:"updated_at"`
	Error       string `json:"error,omitempty"`
}

type progressItem struct {
	mu        sync.Mutex
	status    string
	err       string
	total     int64
	read      int64
	startedAt time.Time
	updatedAt time.Time
	doneAt    time.Time
}

// ProgressStore keeps short-lived progress states keyed by progress_id.
// It is intentionally small and in-memory (educational UI feature).
type ProgressStore struct {
	mu    sync.RWMutex
	items map[string]*progressItem
}

func NewProgressStore() *ProgressStore {
	return &ProgressStore{items: make(map[string]*progressItem, 64)}
}

func (s *ProgressStore) Start(id string, totalBytes int64) {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked(now)
	s.items[id] = &progressItem{
		status:    "processing",
		total:     totalBytes,
		startedAt: now,
		updatedAt: now,
	}
}

func (s *ProgressStore) Add(id string, n int) {
	if n <= 0 {
		return
	}

	s.mu.RLock()
	it := s.items[id]
	s.mu.RUnlock()
	if it == nil {
		return
	}

	it.mu.Lock()
	it.read += int64(n)
	if it.total > 0 && it.read > it.total {
		it.read = it.total
	}
	it.updatedAt = time.Now()
	it.mu.Unlock()
}

func (s *ProgressStore) Done(id string) {
	s.mu.RLock()
	it := s.items[id]
	s.mu.RUnlock()
	if it == nil {
		return
	}

	it.mu.Lock()
	it.status = "done"
	it.updatedAt = time.Now()
	it.doneAt = it.updatedAt
	// если total известен, сделаем "красивые" 100%.
	if it.total > 0 && it.read < it.total {
		it.read = it.total
	}
	it.mu.Unlock()
}

func (s *ProgressStore) Fail(id string, err error) {
	s.mu.RLock()
	it := s.items[id]
	s.mu.RUnlock()
	if it == nil {
		return
	}

	it.mu.Lock()
	it.status = "error"
	if err != nil {
		it.err = err.Error()
	}
	it.updatedAt = time.Now()
	it.doneAt = it.updatedAt
	it.mu.Unlock()
}

func (s *ProgressStore) Get(id string) (CDRProgressResponse, bool) {
	s.mu.RLock()
	it := s.items[id]
	s.mu.RUnlock()
	if it == nil {
		return CDRProgressResponse{}, false
	}

	it.mu.Lock()
	status := it.status
	err := it.err
	read := it.read
	total := it.total
	updated := it.updatedAt
	doneAt := it.doneAt
	it.mu.Unlock()

	// best-effort cleanup: if done long ago, drop it.
	if !doneAt.IsZero() && time.Since(doneAt) > 15*time.Minute {
		s.mu.Lock()
		delete(s.items, id)
		s.mu.Unlock()
		return CDRProgressResponse{}, false
	}

	var pct *int
	if total > 0 {
		p := int((read * 100) / total)
		if p < 0 {
			p = 0
		}
		if p > 100 {
			p = 100
		}
		pct = &p
	}

	return CDRProgressResponse{
		Status:      status,
		ProgressPct: pct,
		ReadBytes:   read,
		TotalBytes:  total,
		UpdatedAt:   updated.UTC().Format(time.RFC3339),
		Error:       err,
	}, true
}

func (s *ProgressStore) cleanupLocked(now time.Time) {
	// cheap cleanup: drop finished items older than TTL.
	const ttl = 15 * time.Minute
	for id, it := range s.items {
		it.mu.Lock()
		doneAt := it.doneAt
		it.mu.Unlock()
		if !doneAt.IsZero() && now.Sub(doneAt) > ttl {
			delete(s.items, id)
		}
	}
}

type countingReader struct {
	r      io.Reader
	onRead func(n int)
}

func newCountingReader(r io.Reader, onRead func(n int)) io.Reader {
	return &countingReader{r: r, onRead: onRead}
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 && c.onRead != nil {
		c.onRead(n)
	}
	return n, err
}

package session

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/observability"
)

// Session stores in-memory conversation state.
type Session struct {
	ID              string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Messages        []map[string]any
	UserTurns       int
	Checkpoints     map[string][]map[string]any
	FileCheckpoints map[string]map[string]*string
}

// Manager manages in-memory sessions.
type Manager struct {
	mu           sync.Mutex
	sessions     map[string]*Session
	forkCounters map[string]int
	storePath    string
	persistent   bool
	obs          *observability.Observer
}

// NewManager creates a session manager.
func NewManager() *Manager {
	return &Manager{
		sessions:     make(map[string]*Session, 8),
		forkCounters: make(map[string]int, 8),
		obs:          observability.Noop(),
	}
}

// SetObserver sets the observability observer for the session manager.
func (m *Manager) SetObserver(obs *observability.Observer) {
	if obs != nil {
		m.obs = obs
	}
}

type persistedState struct {
	Sessions     map[string]*Session `json:"sessions"`
	ForkCounters map[string]int      `json:"fork_counters"`
}

// EnablePersistence enables durable session persistence at the provided path.
// Existing state from disk is loaded immediately when present.
func (m *Manager) EnablePersistence(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(path) == "" {
		m.persistent = false
		m.storePath = ""
		return nil
	}

	m.storePath = path
	m.persistent = true
	return m.loadLocked()
}

// GetOrCreate gets or creates a session.
func (m *Manager) GetOrCreate(id string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id == "" {
		id = "default"
	}
	if s, ok := m.sessions[id]; ok {
		return s
	}
	now := time.Now().UTC()
	s := &Session{
		ID:              id,
		CreatedAt:       now,
		UpdatedAt:       now,
		Checkpoints:     make(map[string][]map[string]any),
		FileCheckpoints: make(map[string]map[string]*string),
	}
	m.sessions[id] = s
	_ = m.saveLocked()
	return s
}

// Get returns an existing session without creating it.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id == "" {
		id = "default"
	}
	s, ok := m.sessions[id]
	return s, ok
}

// Clone clones session history and checkpoints from source to destination.
func (m *Manager) Clone(fromID, toID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fromID == "" || toID == "" {
		return false
	}
	src, ok := m.sessions[fromID]
	if !ok {
		return false
	}
	now := time.Now().UTC()
	dst := &Session{
		ID:              toID,
		CreatedAt:       now,
		UpdatedAt:       now,
		Messages:        cloneMessages(src.Messages),
		UserTurns:       src.UserTurns,
		Checkpoints:     make(map[string][]map[string]any, len(src.Checkpoints)),
		FileCheckpoints: make(map[string]map[string]*string, len(src.FileCheckpoints)),
	}
	for k, cp := range src.Checkpoints {
		dst.Checkpoints[k] = cloneMessages(cp)
	}
	for k, cp := range src.FileCheckpoints {
		dst.FileCheckpoints[k] = cloneFileSnapshot(cp)
	}
	m.sessions[toID] = dst
	_ = m.saveLocked()
	return true
}

// NewForkID creates a deterministic fork ID for a base session ID.
func (m *Manager) NewForkID(base string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if base == "" {
		base = "default"
	}
	m.forkCounters[base]++
	_ = m.saveLocked()
	return base + "#fork-" + itoa(m.forkCounters[base])
}

// Snapshot stores a checkpoint for a user message.
func (m *Manager) Snapshot(id, userMessageID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || userMessageID == "" {
		m.obs.RecordCheckpointOp(context.Background(), "create", "skipped")
		return
	}
	cp := cloneMessages(s.Messages)
	s.Checkpoints[userMessageID] = cp
	touchSession(s)
	_ = m.saveLocked()
	m.obs.RecordCheckpointOp(context.Background(), "create", "ok")
}

// Rewind rewinds to a checkpoint.
func (m *Manager) Rewind(id, userMessageID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		m.obs.RecordCheckpointOp(context.Background(), "restore", "not_found")
		return false
	}
	cp, ok := s.Checkpoints[userMessageID]
	if !ok {
		m.obs.RecordCheckpointOp(context.Background(), "restore", "not_found")
		return false
	}
	s.Messages = cloneMessages(cp)
	touchSession(s)
	_ = m.saveLocked()
	m.obs.RecordCheckpointOp(context.Background(), "restore", "ok")
	return true
}

// SetState replaces the current session message history and user-turn count.
func (m *Manager) SetState(id string, messages []map[string]any, userTurns int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := m.getOrCreateLocked(id)
	s.Messages = cloneMessages(messages)
	s.UserTurns = userTurns
	touchSession(s)
	_ = m.saveLocked()
}

// List returns cloned sessions sorted by last update time, newest first.
func (m *Manager) List() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, cloneSession(s))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

// SnapshotFiles stores a filesystem snapshot rooted at root for a user message.
func (m *Manager) SnapshotFiles(id, userMessageID, root string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok || userMessageID == "" {
		return ErrNoCheckpoint
	}
	if strings.TrimSpace(root) == "" {
		return errors.New("snapshot root is empty")
	}
	files, err := captureFiles(root)
	if err != nil {
		return err
	}
	if s.FileCheckpoints == nil {
		s.FileCheckpoints = make(map[string]map[string]*string, 4)
	}
	s.FileCheckpoints[userMessageID] = files
	touchSession(s)
	return m.saveLocked()
}

// RewindFiles rewinds files rooted at root to a stored snapshot.
func (m *Manager) RewindFiles(id, userMessageID, root string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[id]
	if !ok || strings.TrimSpace(root) == "" {
		return ErrNoCheckpoint
	}
	cp, ok := s.FileCheckpoints[userMessageID]
	if !ok {
		return ErrNoCheckpoint
	}
	if err := restoreFiles(root, cp); err != nil {
		return err
	}
	touchSession(s)
	_ = m.saveLocked()
	return nil
}

func (m *Manager) loadLocked() error {
	if !m.persistent || strings.TrimSpace(m.storePath) == "" {
		return nil
	}
	raw, err := os.ReadFile(m.storePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	var state persistedState
	if err := json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if state.Sessions != nil {
		m.sessions = state.Sessions
	}
	if state.ForkCounters != nil {
		m.forkCounters = state.ForkCounters
	}
	for _, s := range m.sessions {
		if s.CreatedAt.IsZero() {
			s.CreatedAt = time.Now().UTC()
		}
		if s.UpdatedAt.IsZero() {
			s.UpdatedAt = s.CreatedAt
		}
		if s.Checkpoints == nil {
			s.Checkpoints = make(map[string][]map[string]any)
		}
		if s.FileCheckpoints == nil {
			s.FileCheckpoints = make(map[string]map[string]*string)
		}
	}
	return nil
}

func (m *Manager) getOrCreateLocked(id string) *Session {
	if id == "" {
		id = "default"
	}
	if s, ok := m.sessions[id]; ok {
		return s
	}
	now := time.Now().UTC()
	s := &Session{
		ID:              id,
		CreatedAt:       now,
		UpdatedAt:       now,
		Checkpoints:     make(map[string][]map[string]any),
		FileCheckpoints: make(map[string]map[string]*string),
	}
	m.sessions[id] = s
	return s
}

func (m *Manager) saveLocked() error {
	if !m.persistent || strings.TrimSpace(m.storePath) == "" {
		return nil
	}
	state := persistedState{
		Sessions:     m.sessions,
		ForkCounters: m.forkCounters,
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.storePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(m.storePath, raw, 0o644)
}

func captureFiles(root string) (map[string]*string, error) {
	base, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*string, 64)

	err = filepath.WalkDir(base, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(b)
		out[rel] = &s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func restoreFiles(root string, snapshot map[string]*string) error {
	base, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	current, err := captureFiles(base)
	if err != nil {
		return err
	}
	for rel := range current {
		if _, ok := snapshot[rel]; ok {
			continue
		}
		target := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	for rel, content := range snapshot {
		if strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
			return errors.New("invalid snapshot path")
		}
		target := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if content == nil {
			if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			continue
		}
		if err := os.WriteFile(target, []byte(*content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func cloneFileSnapshot(in map[string]*string) map[string]*string {
	if len(in) == 0 {
		return map[string]*string{}
	}
	out := make(map[string]*string, len(in))
	for k, v := range in {
		if v == nil {
			out[k] = nil
			continue
		}
		c := *v
		out[k] = &c
	}
	return out
}

func cloneSession(in *Session) *Session {
	if in == nil {
		return nil
	}
	return &Session{
		ID:              in.ID,
		CreatedAt:       in.CreatedAt,
		UpdatedAt:       in.UpdatedAt,
		Messages:        cloneMessages(in.Messages),
		UserTurns:       in.UserTurns,
		Checkpoints:     cloneCheckpointMap(in.Checkpoints),
		FileCheckpoints: cloneFileCheckpointMap(in.FileCheckpoints),
	}
}

func cloneCheckpointMap(in map[string][]map[string]any) map[string][]map[string]any {
	if len(in) == 0 {
		return map[string][]map[string]any{}
	}
	out := make(map[string][]map[string]any, len(in))
	for k, v := range in {
		out[k] = cloneMessages(v)
	}
	return out
}

func cloneFileCheckpointMap(in map[string]map[string]*string) map[string]map[string]*string {
	if len(in) == 0 {
		return map[string]map[string]*string{}
	}
	out := make(map[string]map[string]*string, len(in))
	for k, v := range in {
		out[k] = cloneFileSnapshot(v)
	}
	return out
}

func touchSession(s *Session) {
	if s == nil {
		return
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	s.UpdatedAt = time.Now().UTC()
}

// cloneMessages makes a shallow clone of message list and message maps.
func cloneMessages(in []map[string]any) []map[string]any {
	out := make([]map[string]any, len(in))
	for i, msg := range in {
		cp := make(map[string]any, len(msg))
		for k, v := range msg {
			cp[k] = v
		}
		out[i] = cp
	}
	return out
}

// Clone clones message history.
func Clone(in []map[string]any) []map[string]any {
	return cloneMessages(in)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + (v % 10))
		v /= 10
	}
	return sign + string(buf[i:])
}

package lmsdk

import (
	"context"
	"fmt"
	"os"

	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/session"
)

// SessionStat contains metadata about a locally persisted SDK session.
type SessionStat struct {
	SessionID           string
	CreatedAt           string
	UpdatedAt           string
	MessageCount        int
	UserTurns           int
	CheckpointCount     int
	FileCheckpointCount int
}

// StatSession returns metadata for a locally persisted SDK session.
func StatSession(ctx context.Context, sessionID string, opts ...Option) (*SessionStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manager, err := loadSessionManager(opts...)
	if err != nil {
		return nil, err
	}

	s, ok := manager.Get(sessionID)
	if !ok {
		return nil, sdkerrors.ErrSessionNotFound
	}
	return buildSessionStat(s), nil
}

func buildSessionStat(s *session.Session) *SessionStat {
	if s == nil {
		return nil
	}
	return &SessionStat{
		SessionID:           s.ID,
		CreatedAt:           s.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		UpdatedAt:           s.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		MessageCount:        len(s.Messages),
		UserTurns:           s.UserTurns,
		CheckpointCount:     len(s.Checkpoints),
		FileCheckpointCount: len(s.FileCheckpoints),
	}
}

func loadSessionManager(opts ...Option) (*session.Manager, error) {
	options := applyAgentOptions(opts)
	path := ""
	if options != nil {
		path = options.SessionStorePath
	}
	if path == "" {
		path = os.Getenv("LM_AGENT_SESSION_STORE_PATH")
	}
	if path == "" {
		return nil, fmt.Errorf("session store path is required: use WithSessionStorePath or LM_AGENT_SESSION_STORE_PATH")
	}

	manager := session.NewManager()
	if err := manager.EnablePersistence(path); err != nil {
		return nil, err
	}
	return manager, nil
}

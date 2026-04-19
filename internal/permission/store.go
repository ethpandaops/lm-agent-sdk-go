package permission

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	policyDirName      = ".lm-agent-sdk-go"
	userPolicyFile     = "permissions.user.json"
	projectPolicyFile  = "permissions.project.json"
	localPolicyFile    = "permissions.local.json"
)

// Policy stores durable permission state.
type Policy struct {
	Mode  *Mode             `json:"mode,omitempty"`
	Rules map[string]Behavior `json:"rules,omitempty"`
}

// Store manages persistent permission policies.
type Store struct {
	userPath    string
	projectPath string
	localPath   string
}

// NewStore creates a policy store rooted at cwd for project/local destinations.
func NewStore(cwd string) *Store {
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}

	userPath := userPolicyFile
	if cfg, err := os.UserConfigDir(); err == nil && cfg != "" {
		userPath = filepath.Join(cfg, policyDirName, userPolicyFile)
	}

	base := filepath.Join(cwd, policyDirName)
	return &Store{
		userPath:    userPath,
		projectPath: filepath.Join(base, projectPolicyFile),
		localPath:   filepath.Join(base, localPolicyFile),
	}
}

// LoadMerged loads user->project->local policies with later values overriding earlier ones.
func (s *Store) LoadMerged() (*Policy, error) {
	out := &Policy{Rules: map[string]Behavior{}}
	paths := []string{s.userPath, s.projectPath, s.localPath}
	for _, p := range paths {
		pol, err := s.loadPolicyFile(p)
		if err != nil {
			return nil, err
		}
		if pol == nil {
			continue
		}
		if pol.Mode != nil {
			mv := *pol.Mode
			out.Mode = &mv
		}
		for k, v := range pol.Rules {
			out.Rules[k] = v
		}
	}
	return out, nil
}

// ApplyPersistentUpdate applies update to a destination-backed policy file.
func (s *Store) ApplyPersistentUpdate(update *Update) error {
	if update == nil || update.Destination == nil {
		return nil
	}
	if *update.Destination == UpdateDestSession {
		return nil
	}

	path, ok := s.pathFor(*update.Destination)
	if !ok {
		return nil
	}

	pol, err := s.loadPolicyFile(path)
	if err != nil {
		return err
	}
	if pol == nil {
		pol = &Policy{Rules: map[string]Behavior{}}
	}
	applyUpdate(pol, update)
	return s.savePolicyFile(path, pol)
}

func (s *Store) pathFor(dest UpdateDestination) (string, bool) {
	switch dest {
	case UpdateDestUserSettings:
		return s.userPath, true
	case UpdateDestProjectSettings:
		return s.projectPath, true
	case UpdateDestLocalSettings:
		return s.localPath, true
	default:
		return "", false
	}
}

func applyUpdate(pol *Policy, update *Update) {
	if pol.Rules == nil {
		pol.Rules = map[string]Behavior{}
	}

	switch update.Type {
	case UpdateTypeSetMode:
		pol.Mode = update.Mode
	case UpdateTypeAddRules:
		behavior := BehaviorAsk
		if update.Behavior != nil {
			behavior = *update.Behavior
		}
		for _, r := range update.Rules {
			if r == nil || r.ToolName == "" {
				continue
			}
			pol.Rules[r.ToolName] = behavior
		}
	case UpdateTypeReplaceRules:
		pol.Rules = map[string]Behavior{}
		behavior := BehaviorAsk
		if update.Behavior != nil {
			behavior = *update.Behavior
		}
		for _, r := range update.Rules {
			if r == nil || r.ToolName == "" {
				continue
			}
			pol.Rules[r.ToolName] = behavior
		}
	case UpdateTypeRemoveRules:
		for _, r := range update.Rules {
			if r == nil || r.ToolName == "" {
				continue
			}
			delete(pol.Rules, r.ToolName)
		}
	}
}

func (s *Store) loadPolicyFile(path string) (*Policy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var p Policy
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if p.Rules == nil {
		p.Rules = map[string]Behavior{}
	}
	return &p, nil
}

func (s *Store) savePolicyFile(path string, p *Policy) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

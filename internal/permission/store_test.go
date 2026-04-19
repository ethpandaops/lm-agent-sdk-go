package permission

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreApplyAndLoadMerged(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)

	allow := BehaviorAllow
	deny := BehaviorDeny
	mode := ModePlan

	// user rule allow
	if err := s.ApplyPersistentUpdate(&Update{
		Type:        UpdateTypeAddRules,
		Behavior:    &allow,
		Destination: ptrDest(UpdateDestUserSettings),
		Rules: []*RuleValue{
			{ToolName: "tool.user"},
		},
	}); err != nil {
		t.Fatalf("apply user update: %v", err)
	}

	// project overrides same tool to deny
	if err := s.ApplyPersistentUpdate(&Update{
		Type:        UpdateTypeAddRules,
		Behavior:    &deny,
		Destination: ptrDest(UpdateDestProjectSettings),
		Rules: []*RuleValue{
			{ToolName: "tool.user"},
		},
	}); err != nil {
		t.Fatalf("apply project update: %v", err)
	}

	// local mode
	if err := s.ApplyPersistentUpdate(&Update{
		Type:        UpdateTypeSetMode,
		Mode:        &mode,
		Destination: ptrDest(UpdateDestLocalSettings),
	}); err != nil {
		t.Fatalf("apply local mode: %v", err)
	}

	merged, err := s.LoadMerged()
	if err != nil {
		t.Fatalf("load merged: %v", err)
	}
	if merged.Mode == nil || *merged.Mode != ModePlan {
		t.Fatalf("expected mode plan, got %+v", merged.Mode)
	}
	if merged.Rules["tool.user"] != BehaviorDeny {
		t.Fatalf("expected project override deny, got %v", merged.Rules["tool.user"])
	}
}

func TestStoreSessionDestinationNotPersisted(t *testing.T) {
	tmp := t.TempDir()
	s := NewStore(tmp)
	allow := BehaviorAllow

	if err := s.ApplyPersistentUpdate(&Update{
		Type:        UpdateTypeAddRules,
		Behavior:    &allow,
		Destination: ptrDest(UpdateDestSession),
		Rules: []*RuleValue{
			{ToolName: "tool.session"},
		},
	}); err != nil {
		t.Fatalf("apply session update: %v", err)
	}

	files, err := os.ReadDir(filepath.Join(tmp, policyDirName))
	if err == nil && len(files) > 0 {
		t.Fatalf("expected no persisted files for session destination")
	}
}

func ptrDest(d UpdateDestination) *UpdateDestination { return &d }

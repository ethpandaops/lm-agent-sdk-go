// Package permission provides permission handling types.
package permission

import "context"

// Mode represents different permission handling modes.
type Mode string

const (
	// ModeDefault uses standard permission prompts.
	ModeDefault Mode = "default"
	// ModeAcceptEdits automatically accepts file edits.
	ModeAcceptEdits Mode = "acceptEdits"
	// ModePlan enables plan mode for implementation planning.
	ModePlan Mode = "plan"
	// ModeBypassPermissions bypasses all permission checks.
	ModeBypassPermissions Mode = "bypassPermissions"
)

// UpdateType represents the type of permission update.
type UpdateType string

const (
	UpdateTypeAddRules          UpdateType = "addRules"
	UpdateTypeReplaceRules      UpdateType = "replaceRules"
	UpdateTypeRemoveRules       UpdateType = "removeRules"
	UpdateTypeSetMode           UpdateType = "setMode"
	UpdateTypeAddDirectories    UpdateType = "addDirectories"
	UpdateTypeRemoveDirectories UpdateType = "removeDirectories"
)

// UpdateDestination represents where permission updates are stored.
type UpdateDestination string

const (
	UpdateDestUserSettings    UpdateDestination = "userSettings"
	UpdateDestProjectSettings UpdateDestination = "projectSettings"
	UpdateDestLocalSettings   UpdateDestination = "localSettings"
	UpdateDestSession         UpdateDestination = "session"
)

// Behavior represents the permission behavior for a rule.
type Behavior string

const (
	BehaviorAllow Behavior = "allow"
	BehaviorDeny  Behavior = "deny"
	BehaviorAsk   Behavior = "ask"
)

// RuleValue represents a permission rule.
type RuleValue struct {
	ToolName    string
	RuleContent *string
}

// Update represents a permission update request.
type Update struct {
	Type        UpdateType
	Rules       []*RuleValue
	Behavior    *Behavior
	Mode        *Mode
	Directories []string
	Destination *UpdateDestination
}

// ToDict converts the Update to a map.
func (p *Update) ToDict() map[string]any {
	result := make(map[string]any, 6)
	result["type"] = string(p.Type)

	if p.Destination != nil {
		result["destination"] = string(*p.Destination)
	}

	if len(p.Rules) > 0 {
		rules := make([]map[string]any, len(p.Rules))
		for i, rule := range p.Rules {
			ruleMap := map[string]any{"toolName": rule.ToolName}
			if rule.RuleContent != nil {
				ruleMap["ruleContent"] = *rule.RuleContent
			}
			rules[i] = ruleMap
		}
		result["rules"] = rules
	}

	if p.Behavior != nil {
		result["behavior"] = string(*p.Behavior)
	}
	if p.Mode != nil {
		result["mode"] = string(*p.Mode)
	}
	if len(p.Directories) > 0 {
		result["directories"] = p.Directories
	}

	return result
}

// Context provides context for tool permission callbacks.
type Context struct {
	Suggestions []*Update
}

// Result is the interface for permission decision results.
type Result interface {
	GetBehavior() string
}

// ResultAllow represents an allow decision.
type ResultAllow struct {
	Behavior           string
	UpdatedInput       map[string]any
	UpdatedPermissions []*Update
}

// GetBehavior implements Result.
func (p *ResultAllow) GetBehavior() string { return "allow" }

// ResultDeny represents a deny decision.
type ResultDeny struct {
	Behavior  string
	Message   string
	Interrupt bool
}

// GetBehavior implements Result.
func (p *ResultDeny) GetBehavior() string { return "deny" }

// Callback is called before each tool use for permission checking.
type Callback func(
	ctx context.Context,
	toolName string,
	input map[string]any,
	permCtx *Context,
) (Result, error)

package config

// ToolsPreset represents a preset configuration for available tools.
type ToolsPreset struct {
	Type   string `json:"type"`   // "preset"
	Preset string `json:"preset"` // backend-defined preset identifier
}

// SystemPromptPreset defines a system prompt preset configuration.
type SystemPromptPreset struct {
	Type   string  `json:"type"`   // "preset"
	Preset string  `json:"preset"` // backend-defined preset identifier
	Append *string `json:"append,omitempty"`
}

// PluginConfig configures a plugin to load.
type PluginConfig struct {
	Type string `json:"type"` // "local"
	Path string `json:"path"`
}

// ToolsConfig is an interface for configuring available tools.
type ToolsConfig interface {
	toolsConfig()
}

// ToolsList is a list of tool names to make available.
type ToolsList []string

func (ToolsList) toolsConfig()    {}
func (*ToolsPreset) toolsConfig() {}

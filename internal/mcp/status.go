package mcp

// ServerStatus represents the connection status of a single MCP server.
type ServerStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// Status represents the connection status of all configured MCP servers.
type Status struct {
	MCPServers []ServerStatus `json:"mcpServers"`
}

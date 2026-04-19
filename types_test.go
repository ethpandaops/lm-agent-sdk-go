package lmsdk

import "testing"

func TestRenamedPublicTypesCompile(t *testing.T) {
	var _ *AgentOptions
	var _ SDKError
	var _ Model
	var _ ModelInfo
	var _ ModelListResponse
	var _ UserInputCallback
	var _ SessionStat
}

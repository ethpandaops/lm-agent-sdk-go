//go:build integration

package integration_test

import (
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

// MaxBudgetUSD enforcement is untestable against LM Studio: local models have
// no pricing metadata, so costs are always zero and the cap never fires.
// The option itself remains in the SDK for users who plug custom pricing via
// hooks or external cost tracking.

func TestMaxBudgetUSD_NormalBudget(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithMaxBudgetUSD(1.0))

	result := collectResult(t, lmsdk.Query(ctx, promptText("Reply with the word: budget."), opts...))
	if result.Result == nil {
		t.Fatal("expected result with normal budget")
	}
}

func TestQueryWithCostTracking(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	result := collectResult(t, lmsdk.Query(ctx, promptText("Reply with the word: cost."), integrationOptions()...))

	if result.Usage != nil {
		if result.Usage.InputTokens == 0 && result.Usage.OutputTokens == 0 {
			t.Log("usage reported but tokens are zero (may be a free model)")
		}
	}

	// TotalCostUSD may be nil for free models.
	if result.TotalCostUSD != nil && *result.TotalCostUSD < 0 {
		t.Fatalf("unexpected negative cost: %v", *result.TotalCostUSD)
	}
}

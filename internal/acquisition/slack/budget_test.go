package slack

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFileSlackBudgetStoreRestoresEveryCounterAndOriginalWallClock(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	scope := SlackBudgetScope{WorkspaceID: "T-proof", ChannelID: "C-proof", Oldest: "100.000001", Latest: "199.000001"}
	budgets := SlackHTTPBudgets{MaximumRequests: 3, MaximumItems: 5, MaximumBytes: 100, MaximumRetries: 1, MaximumRetryAfter: time.Second, MaximumWallTime: time.Minute, MaximumCostMicrounits: 3}
	root := t.TempDir()
	first, err := NewFileSlackBudgetStore(root, scope, budgets, now)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := first.ReserveRequest(context.Background(), now, 3, 60)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SettleRequest(context.Background(), reservation, 2, 40); err != nil {
		t.Fatal(err)
	}
	if err := first.ReserveRetry(context.Background(), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewFileSlackBudgetStore(root, scope, budgets, now.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	usage, err := restarted.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if usage.StartedAt != now.Format(time.RFC3339Nano) || usage.Requests != 1 || usage.Items != 2 || usage.Bytes != 40 || usage.Retries != 1 || usage.CostMicrounits != 1 {
		t.Fatalf("restart reset durable accounting: %+v", usage)
	}
	if err := restarted.ReserveRetry(context.Background(), now.Add(31*time.Second)); !errors.Is(err, ErrSlackRateLimited) {
		t.Fatalf("restart reset retry budget: %v", err)
	}
	if _, err := restarted.ReserveRequest(context.Background(), now.Add(31*time.Second), 4, 1); !errors.Is(err, ErrSlackBudgetExceeded) {
		t.Fatalf("restart reset item budget: %v", err)
	}
	if _, err := restarted.ReserveRequest(context.Background(), now.Add(31*time.Second), 1, 61); !errors.Is(err, ErrSlackBudgetExceeded) {
		t.Fatalf("restart reset byte budget: %v", err)
	}
	if _, err := restarted.ReserveRequest(context.Background(), now.Add(61*time.Second), 0, 1); !errors.Is(err, ErrSlackBudgetExceeded) {
		t.Fatalf("restart reset original wall-clock budget: %v", err)
	}
}

func TestFileSlackBudgetStoreFailSafeReservationsSurviveCrashAndRequestCostCaps(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	scope := SlackBudgetScope{WorkspaceID: "T-proof", ChannelID: "C-proof", Oldest: "100.000001", Latest: "199.000001"}
	budgets := SlackHTTPBudgets{MaximumRequests: 2, MaximumItems: 2, MaximumBytes: 20, MaximumRetries: 1, MaximumRetryAfter: time.Second, MaximumWallTime: time.Hour, MaximumCostMicrounits: 2}
	root := t.TempDir()
	first, err := NewFileSlackBudgetStore(root, scope, budgets, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.ReserveRequest(context.Background(), now, 2, 20); err != nil {
		t.Fatal(err)
	}
	// No settlement simulates a crash after durable reservation and before a
	// trustworthy response was accounted.
	restarted, err := NewFileSlackBudgetStore(root, scope, budgets, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	usage, err := restarted.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if usage.Requests != 1 || usage.Items != 2 || usage.Bytes != 20 || usage.CostMicrounits != 1 || len(usage.Pending) != 1 {
		t.Fatalf("crash reservation was not fail-safe: %+v", usage)
	}
	if _, err := restarted.ReserveRequest(context.Background(), now.Add(time.Second), 0, 1); !errors.Is(err, ErrSlackBudgetExceeded) {
		t.Fatalf("pending crash reservation did not close the byte budget: %v", err)
	}
	data, err := os.ReadFile(restarted.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"xoxp-", "https://", "message_text", "raw_cursor"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("budget ledger persisted source content or credentials %q", forbidden)
		}
	}
}

func TestFileSlackBudgetStoreRejectsPolicyOrScopeReset(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	scope := SlackBudgetScope{WorkspaceID: "T-proof", ChannelID: "C-proof", Oldest: "100.000001", Latest: "199.000001"}
	budgets := DefaultSlackHTTPBudgets()
	root := t.TempDir()
	if _, err := NewFileSlackBudgetStore(root, scope, budgets, now); err != nil {
		t.Fatal(err)
	}
	changed := budgets
	changed.MaximumRequests--
	if _, err := NewFileSlackBudgetStore(root, scope, changed, now); err == nil {
		t.Fatal("same frozen scope adopted a changed budget policy")
	}
}

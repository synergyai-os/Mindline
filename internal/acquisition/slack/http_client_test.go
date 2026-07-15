package slack

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/synergyai-os/Mindline/internal/integrations"
)

type slackRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn slackRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type partialSlackBody struct{ sent bool }

func (body *partialSlackBody) Read(target []byte) (int, error) {
	if body.sent {
		return 0, errors.New("synthetic partial body failure")
	}
	body.sent = true
	return copy(target, "partial-response"), nil
}

func slackResponse(status int, body, scopes string, retryAfter string) *http.Response {
	header := http.Header{}
	if scopes != "" {
		header.Set("X-OAuth-Scopes", scopes)
	}
	if retryAfter != "" {
		header.Set("Retry-After", retryAfter)
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}

func testSlackHTTPClient(t *testing.T, budgets SlackHTTPBudgets, roundTrip slackRoundTripFunc) *SlackHTTPClient {
	t.Helper()
	client, err := NewSlackHTTPClient([]byte("xoxp-synthetic-session-token"), budgets)
	if err != nil {
		t.Fatal(err)
	}
	client.client = &http.Client{Transport: roundTrip, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect") }}
	client.sleep = func(context.Context, time.Duration) error { return nil }
	return client
}

func TestSlackHTTPClientPinsOriginMethodsScopesAndFilePolicy(t *testing.T) {
	budgets := DefaultSlackHTTPBudgets()
	requests := 0
	client := testSlackHTTPClient(t, budgets, func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.Scheme != "https" || request.URL.Host != "slack.com" || request.Header.Get("Authorization") == "" || request.Method != http.MethodPost {
			t.Fatalf("unsafe Slack request: %+v", request)
		}
		scopes := "channels:history,files:read"
		switch request.URL.Path {
		case "/api/auth.test":
			return slackResponse(200, `{"ok":true,"team_id":"T-proof","extra":"ignored"}`, scopes, ""), nil
		case "/api/conversations.history":
			return slackResponse(200, `{"ok":true,"messages":[{"ts":"120.000001","text":"https://example.com","reply_count":0,"files":[{"id":"F1","mode":"hosted","url_private":"https://files.slack.com/private"}]}],"response_metadata":{"next_cursor":""}}`, scopes, ""), nil
		default:
			t.Fatalf("unexpected method path %s", request.URL.Path)
			return nil, nil
		}
	})
	defer client.Close()
	workspace, err := client.Probe(context.Background())
	if err != nil || workspace != "T-proof" {
		t.Fatalf("probe failed: %s %v", workspace, err)
	}
	page, err := client.History(context.Background(), "C-proof", "100.000001", "199.000001", "", 200)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].FileCount != 1 || page.Messages[0].PrivateFileCount != 1 || requests != 2 {
		t.Fatalf("history/file accounting failed: %+v err=%v requests=%d", page, err, requests)
	}
}

func TestSlackHTTPClientBoundsRateLimitRevocationAndAmbientProxy(t *testing.T) {
	budgets := DefaultSlackHTTPBudgets()
	budgets.MaximumRetries = 1
	budgets.MaximumRetryAfter = time.Second
	rateCalls := 0
	client := testSlackHTTPClient(t, budgets, func(*http.Request) (*http.Response, error) {
		rateCalls++
		return slackResponse(429, `{}`, "channels:history", "1"), nil
	})
	if _, err := client.Probe(context.Background()); !errors.Is(err, ErrSlackRateLimited) || rateCalls != 2 {
		t.Fatalf("429 storm was not bounded: calls=%d err=%v", rateCalls, err)
	}
	client.Close()

	revoked := testSlackHTTPClient(t, DefaultSlackHTTPBudgets(), func(*http.Request) (*http.Response, error) {
		return slackResponse(200, `{"ok":false,"error":"token_revoked"}`, "channels:history", ""), nil
	})
	if _, err := revoked.Probe(context.Background()); !errors.Is(err, ErrSlackRevoked) {
		t.Fatalf("revocation not terminal: %v", err)
	}
	if _, err := revoked.Probe(context.Background()); !errors.Is(err, ErrSlackRevoked) {
		t.Fatalf("terminal revocation was retried: %v", err)
	}

	production, err := NewSlackHTTPClient([]byte("xoxp-synthetic-session-token"), DefaultSlackHTTPBudgets())
	if err != nil {
		t.Fatal(err)
	}
	if transport := production.client.Transport.(*http.Transport); transport.Proxy != nil {
		t.Fatal("ambient proxy was enabled")
	}
	production.Close()
}

func TestSlackHTTPClientRejectsWriteScopeAndOversizedResponse(t *testing.T) {
	client := testSlackHTTPClient(t, DefaultSlackHTTPBudgets(), func(*http.Request) (*http.Response, error) {
		return slackResponse(200, `{"ok":true,"team_id":"T"}`, "channels:history,chat:write", ""), nil
	})
	if _, err := client.Probe(context.Background()); err == nil {
		t.Fatal("write-capable Slack scope was accepted")
	}
	client.Close()
	if _, err := readSlackBody(strings.NewReader(strings.Repeat("x", 1025)), 1024); !errors.Is(err, ErrSlackBudgetExceeded) {
		t.Fatalf("oversized body accepted: %v", err)
	}
}

func TestLeasedSlackHTTPClientRechecksSessionLeaseBeforeEveryAttempt(t *testing.T) {
	registry := integrations.NewSessionRegistry(integrations.RegistryOptions{})
	defer registry.Shutdown()
	identity := integrations.VerifiedIdentity{Provider: "slack", WorkspaceID: "T-proof", ChannelID: "C-proof", CapabilityVersion: WebAPIAdapterVersion}
	ref, _, err := registry.Register(integrations.LeaseOptions{
		Kind: integrations.ConnectionSlackWebAPI, Secret: []byte("xoxp-synthetic-session-token"),
		Identity: identity,
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewLeasedSlackHTTPClient(registry, ref, identity, DefaultSlackHTTPBudgets())
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	client.client = &http.Client{Transport: slackRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.Header.Get("Authorization") != "Bearer xoxp-synthetic-session-token" {
			t.Fatal("leased credential was not applied inside the request boundary")
		}
		return slackResponse(200, `{"ok":true,"team_id":"T-proof"}`, "channels:history", ""), nil
	})}
	if workspace, err := client.Probe(context.Background()); err != nil || workspace != "T-proof" {
		t.Fatalf("leased probe failed: workspace=%q err=%v", workspace, err)
	}
	if err := registry.Revoke(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Probe(context.Background()); !errors.Is(err, integrations.ErrLeaseRevoked) || requests != 1 {
		t.Fatalf("revoked lease reached the network: requests=%d err=%v", requests, err)
	}
}

func TestDurablyBudgetedLeasedClientCannotResetRequestOrCostBudgetOnRestart(t *testing.T) {
	registry := integrations.NewSessionRegistry(integrations.RegistryOptions{})
	defer registry.Shutdown()
	identity := integrations.VerifiedIdentity{Provider: "slack", WorkspaceID: "T-proof", ChannelID: "C-proof", CapabilityVersion: WebAPIAdapterVersion}
	ref, _, err := registry.Register(integrations.LeaseOptions{
		Kind: integrations.ConnectionSlackWebAPI, Secret: []byte("xoxp-synthetic-session-token"),
		Identity: identity,
	})
	if err != nil {
		t.Fatal(err)
	}
	budgets := DefaultSlackHTTPBudgets()
	budgets.MaximumRequests = 2
	budgets.MaximumCostMicrounits = 2
	now := time.Now().UTC()
	scope := SlackBudgetScope{WorkspaceID: "T-proof", ChannelID: "C-proof", Oldest: "100.000001", Latest: "199.000001"}
	root := t.TempDir()
	requests := 0
	newClient := func(started time.Time) *SlackHTTPClient {
		store, err := NewFileSlackBudgetStore(root, scope, budgets, started)
		if err != nil {
			t.Fatal(err)
		}
		client, err := NewDurablyBudgetedLeasedSlackHTTPClient(registry, ref, identity, budgets, store)
		if err != nil {
			t.Fatal(err)
		}
		client.now = func() time.Time { return started }
		client.client = &http.Client{Transport: slackRoundTripFunc(func(*http.Request) (*http.Response, error) {
			requests++
			return slackResponse(200, `{"ok":true,"team_id":"T-proof"}`, "channels:history", ""), nil
		})}
		return client
	}
	first := newClient(now)
	if _, err := first.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	first.Close()
	second := newClient(now.Add(time.Second))
	defer second.Close()
	if _, err := second.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Probe(context.Background()); !errors.Is(err, ErrSlackBudgetExceeded) || requests != 2 {
		t.Fatalf("restart reset request/cost authority or reached network: requests=%d err=%v", requests, err)
	}
}

func TestAmbiguousSlackBodiesRetainFailSafeBudgetAcrossRestart(t *testing.T) {
	for _, fixture := range []struct {
		name string
		body func() io.ReadCloser
	}{
		{name: "oversized", body: func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(strings.Repeat("x", int(maximumSlackResponseBytes)+1)))
		}},
		{name: "partial-read", body: func() io.ReadCloser { return io.NopCloser(&partialSlackBody{}) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			now := time.Now().UTC()
			budgets := DefaultSlackHTTPBudgets()
			budgets.MaximumBytes = maximumSlackResponseBytes
			scope := SlackBudgetScope{WorkspaceID: "T-proof", ChannelID: "C-proof", Oldest: "100.000001", Latest: "199.000001"}
			root := t.TempDir()
			store, err := NewFileSlackBudgetStore(root, scope, budgets, now)
			if err != nil {
				t.Fatal(err)
			}
			networkCalls := 0
			first, err := NewSlackHTTPClient([]byte("xoxp-synthetic-session-token"), budgets)
			if err != nil {
				t.Fatal(err)
			}
			first.budget = store
			first.now = func() time.Time { return now }
			first.client = &http.Client{Transport: slackRoundTripFunc(func(*http.Request) (*http.Response, error) {
				networkCalls++
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"X-Oauth-Scopes": []string{"channels:history"}}, Body: fixture.body()}, nil
			})}
			if _, err := first.Probe(context.Background()); err == nil {
				t.Fatal("ambiguous response unexpectedly succeeded")
			}
			first.Close()

			restarted, err := NewFileSlackBudgetStore(root, scope, budgets, now.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			usage, err := restarted.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if usage.Bytes != maximumSlackResponseBytes || len(usage.Pending) != 1 {
				t.Fatalf("ambiguous body released fail-safe reservation: %+v", usage)
			}
			second, err := NewSlackHTTPClient([]byte("xoxp-synthetic-session-token"), budgets)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			second.budget = restarted
			second.now = func() time.Time { return now.Add(time.Second) }
			second.client = first.client
			if _, err := second.Probe(context.Background()); !errors.Is(err, ErrSlackBudgetExceeded) || networkCalls != 1 {
				t.Fatalf("restart reset ambiguous body reservation or reached network: calls=%d err=%v", networkCalls, err)
			}
		})
	}
}

func TestDurableWallDeadlineClampsEverySocketAfterRestart(t *testing.T) {
	startedAt := time.Now().UTC().Add(-30 * time.Second)
	budgets := DefaultSlackHTTPBudgets()
	budgets.MaximumWallTime = time.Minute
	scope := SlackBudgetScope{WorkspaceID: "T-proof", ChannelID: "C-proof", Oldest: "100.000001", Latest: "199.000001"}
	root := t.TempDir()
	if _, err := NewFileSlackBudgetStore(root, scope, budgets, startedAt); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewFileSlackBudgetStore(root, scope, budgets, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewSlackHTTPClient([]byte("xoxp-synthetic-session-token"), budgets)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	client.budget = restarted
	client.now = time.Now
	expected := startedAt.Add(budgets.MaximumWallTime)
	var observed time.Time
	client.client = &http.Client{Transport: slackRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		deadline, ok := request.Context().Deadline()
		if !ok {
			t.Fatal("Slack socket did not receive a cumulative wall deadline")
		}
		observed = deadline
		return slackResponse(200, `{"ok":true,"team_id":"T-proof"}`, "channels:history", ""), nil
	})}
	if _, err := client.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !observed.Equal(expected) {
		t.Fatalf("socket received a fresh timeout instead of the durable deadline: observed=%s expected=%s", observed, expected)
	}
}

package slack

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/synergyai-os/Mindline/internal/integrations"
)

const SlackAPIOrigin = "https://slack.com/api/"

var (
	ErrSlackBudgetExceeded = errors.New("Slack Web API budget exceeded")
	ErrSlackRevoked        = errors.New("Slack authorization revoked")
	ErrSlackRateLimited    = errors.New("Slack Web API rate-limit budget exceeded")
	ErrSlackChannelAccess  = errors.New("Slack channel access unavailable")
)

type SlackHTTPBudgets struct {
	MaximumRequests       int
	MaximumItems          int
	MaximumBytes          int64
	MaximumRetries        int
	MaximumRetryAfter     time.Duration
	MaximumWallTime       time.Duration
	MaximumCostMicrounits int64
}

func DefaultSlackHTTPBudgets() SlackHTTPBudgets {
	return SlackHTTPBudgets{MaximumRequests: 20_000, MaximumItems: 250_000, MaximumBytes: 256 << 20, MaximumRetries: 8, MaximumRetryAfter: 30 * time.Second, MaximumWallTime: 20 * time.Minute, MaximumCostMicrounits: 20_000}
}

type SlackHTTPClient struct {
	mu        sync.Mutex
	token     []byte
	useSecret func(context.Context, func(context.Context, []byte) error) error
	client    *http.Client
	budgets   SlackHTTPBudgets
	budget    slackBudgetStore
	now       func() time.Time
	sleep     func(context.Context, time.Duration) error
	terminal  error
}

func NewSlackHTTPClient(token []byte, budgets SlackHTTPBudgets) (*SlackHTTPClient, error) {
	if len(token) < 16 {
		return nil, errors.New("invalid Slack Web API connection or budgets")
	}
	result, err := newSlackHTTPClient(budgets, newMemorySlackBudgetStore(budgets, time.Now().UTC()))
	if err != nil {
		return nil, err
	}
	result.token = append([]byte(nil), token...)
	result.useSecret = func(ctx context.Context, call func(context.Context, []byte) error) error {
		result.mu.Lock()
		if result.terminal != nil || len(result.token) == 0 {
			err := result.terminal
			if err == nil {
				err = ErrSlackRevoked
			}
			result.mu.Unlock()
			return err
		}
		secret := append([]byte(nil), result.token...)
		result.mu.Unlock()
		defer zeroSlackSecret(secret)
		return call(ctx, secret)
	}
	return result, nil
}

// NewLeasedSlackHTTPClient is the production credential boundary. Every HTTP
// attempt re-enters Registry.Use, so idle expiry, absolute expiry, identity
// drift, disconnect, and in-flight revocation are enforced before the socket.
func NewLeasedSlackHTTPClient(registry *integrations.Registry, ref integrations.SessionRef, identity integrations.VerifiedIdentity, budgets SlackHTTPBudgets) (*SlackHTTPClient, error) {
	if registry == nil || ref == "" || identity.Provider != "slack" || identity.WorkspaceID == "" || identity.ChannelID == "" || identity.CapabilityVersion != WebAPIAdapterVersion {
		return nil, errors.New("invalid leased Slack Web API connection")
	}
	result, err := newSlackHTTPClient(budgets, newMemorySlackBudgetStore(budgets, time.Now().UTC()))
	if err != nil {
		return nil, err
	}
	result.useSecret = func(ctx context.Context, call func(context.Context, []byte) error) error {
		return registry.Use(ctx, ref, identity, func(callContext context.Context, secret []byte) error {
			err := call(callContext, secret)
			if errors.Is(err, ErrSlackRevoked) {
				return errors.Join(err, integrations.ErrCredentialRejected)
			}
			return err
		})
	}
	return result, nil
}

// NewDurablyBudgetedLeasedSlackHTTPClient binds every socket attempt to an
// owner-only, scope-bound budget ledger that survives process restart.
func NewDurablyBudgetedLeasedSlackHTTPClient(registry *integrations.Registry, ref integrations.SessionRef, identity integrations.VerifiedIdentity, budgets SlackHTTPBudgets, store *FileSlackBudgetStore) (*SlackHTTPClient, error) {
	if registry == nil || store == nil || ref == "" || identity.Provider != "slack" || identity.WorkspaceID == "" || identity.ChannelID == "" || identity.CapabilityVersion != WebAPIAdapterVersion || store.scope.WorkspaceID != identity.WorkspaceID || store.scope.ChannelID != identity.ChannelID || store.budgets != budgets {
		return nil, errors.New("invalid durably budgeted Slack Web API connection")
	}
	result, err := newSlackHTTPClient(budgets, store)
	if err != nil {
		return nil, err
	}
	result.useSecret = func(ctx context.Context, call func(context.Context, []byte) error) error {
		return registry.Use(ctx, ref, identity, func(callContext context.Context, secret []byte) error {
			err := call(callContext, secret)
			if errors.Is(err, ErrSlackRevoked) {
				return errors.Join(err, integrations.ErrCredentialRejected)
			}
			return err
		})
	}
	return result, nil
}

func validateSlackHTTPBudgets(budgets SlackHTTPBudgets) error {
	if budgets.MaximumRequests <= 0 || budgets.MaximumItems <= 0 || budgets.MaximumBytes <= 0 || budgets.MaximumRetries < 0 || budgets.MaximumRetryAfter <= 0 || budgets.MaximumWallTime <= 0 || budgets.MaximumCostMicrounits <= 0 {
		return errors.New("invalid Slack Web API budgets")
	}
	return nil
}

func newSlackHTTPClient(budgets SlackHTTPBudgets, budget slackBudgetStore) (*SlackHTTPClient, error) {
	if err := validateSlackHTTPBudgets(budgets); err != nil || budget == nil {
		return nil, errors.New("invalid Slack Web API budgets")
	}
	transport := &http.Transport{
		Proxy:             nil,
		DialContext:       (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true, MaxIdleConns: 4, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}
	result := &SlackHTTPClient{budgets: budgets, budget: budget, now: time.Now}
	result.client = &http.Client{Transport: transport, Timeout: budgets.MaximumWallTime, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("Slack API redirects are forbidden") }}
	result.sleep = func(ctx context.Context, delay time.Duration) error {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
	return result, nil
}

func (client *SlackHTTPClient) Close() {
	client.mu.Lock()
	defer client.mu.Unlock()
	for index := range client.token {
		client.token[index] = 0
	}
	client.token = nil
	client.terminal = ErrSlackRevoked
	if transport, ok := client.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

func (client *SlackHTTPClient) Probe(ctx context.Context) (string, error) {
	var response struct {
		slackEnvelope
		TeamID string `json:"team_id"`
	}
	if err := client.call(ctx, "auth.test", nil, &response, 0, nil); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.TeamID) == "" {
		return "", errors.New("Slack auth.test omitted team identity")
	}
	return response.TeamID, nil
}

func (client *SlackHTTPClient) History(ctx context.Context, channel, oldest, latest, cursor string, limit int) (WebAPIPage, error) {
	values := url.Values{"channel": {channel}, "oldest": {oldest}, "latest": {latest}, "inclusive": {"true"}, "limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		values.Set("cursor", cursor)
	}
	return client.messagePage(ctx, "conversations.history", values)
}

func (client *SlackHTTPClient) Replies(ctx context.Context, channel, thread, oldest, latest, cursor string, limit int) (WebAPIPage, error) {
	values := url.Values{"channel": {channel}, "ts": {thread}, "oldest": {oldest}, "latest": {latest}, "inclusive": {"true"}, "limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		values.Set("cursor", cursor)
	}
	return client.messagePage(ctx, "conversations.replies", values)
}

func (client *SlackHTTPClient) messagePage(ctx context.Context, method string, values url.Values) (WebAPIPage, error) {
	var response struct {
		slackEnvelope
		Messages []struct {
			Timestamp string `json:"ts"`
			Edited    *struct {
				Timestamp string `json:"ts"`
			} `json:"edited"`
			ThreadTimestamp string `json:"thread_ts"`
			Text            string `json:"text"`
			ReplyCount      int    `json:"reply_count"`
			Subtype         string `json:"subtype"`
			Files           []struct {
				ID         string `json:"id"`
				Mode       string `json:"mode"`
				URLPrivate string `json:"url_private"`
			} `json:"files"`
		} `json:"messages"`
		ResponseMetadata struct {
			NextCursor string `json:"next_cursor"`
		} `json:"response_metadata"`
	}
	limit, _ := strconv.Atoi(values.Get("limit"))
	if err := client.call(ctx, method, values, &response, limit, func() int { return len(response.Messages) }); err != nil {
		return WebAPIPage{}, err
	}
	page := WebAPIPage{NextCursor: strings.TrimSpace(response.ResponseMetadata.NextCursor)}
	for _, message := range response.Messages {
		converted := WebAPIMessage{Timestamp: message.Timestamp, ThreadTimestamp: message.ThreadTimestamp, Text: message.Text, ReplyCount: message.ReplyCount, Subtype: message.Subtype, FileCount: len(message.Files)}
		if message.Edited != nil {
			converted.RevisionTimestamp = strings.TrimSpace(message.Edited.Timestamp)
		}
		for _, file := range message.Files {
			if file.URLPrivate != "" || file.Mode != "external" {
				converted.PrivateFileCount++
			}
		}
		page.Messages = append(page.Messages, converted)
	}
	return page, nil
}

type slackEnvelope struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func (client *SlackHTTPClient) call(ctx context.Context, method string, values url.Values, target any, maximumItems int, itemCount func() int) error {
	if method != "auth.test" && method != "conversations.history" && method != "conversations.replies" {
		return errors.New("Slack method is not allowlisted")
	}
	endpoint, err := url.Parse(SlackAPIOrigin + method)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host != "slack.com" {
		return errors.New("Slack origin mismatch")
	}
	for attempt := 0; ; attempt++ {
		deadline, deadlineErr := client.budget.Deadline(ctx)
		if deadlineErr != nil {
			return deadlineErr
		}
		reservation, reserveErr := client.budget.ReserveRequest(ctx, client.now().UTC(), maximumItems, maximumSlackResponseBytes)
		if reserveErr != nil {
			return reserveErr
		}
		settle := func(items int, bytes int64) error {
			return client.budget.SettleRequest(ctx, reservation, items, bytes)
		}
		if err := ctx.Err(); err != nil {
			_ = settle(0, 0)
			return err
		}
		body := ""
		if values != nil {
			body = values.Encode()
		}
		var statusCode int
		var responseHeader http.Header
		var data []byte
		socketAttempted := false
		if client.useSecret == nil {
			_ = settle(0, 0)
			return ErrSlackRevoked
		}
		err = client.useSecret(ctx, func(callContext context.Context, secret []byte) error {
			requestContext, cancel := context.WithDeadline(callContext, deadline)
			defer cancel()
			request, requestErr := http.NewRequestWithContext(requestContext, http.MethodPost, endpoint.String(), strings.NewReader(body))
			if requestErr != nil {
				return requestErr
			}
			request.Header.Set("Authorization", "Bearer "+string(secret))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("User-Agent", "Mindline/SlackSourceAdapter-v1")
			socketAttempted = true
			response, requestErr := client.client.Do(request)
			if requestErr != nil {
				return requestErr
			}
			defer response.Body.Close()
			data, requestErr = readSlackBody(response.Body, maximumSlackResponseBytes)
			if requestErr != nil {
				return requestErr
			}
			statusCode = response.StatusCode
			responseHeader = response.Header.Clone()
			return nil
		})
		if err != nil {
			if !socketAttempted {
				if settleErr := settle(0, 0); settleErr != nil {
					return settleErr
				}
			}
			// Once a socket attempt begins, a transport failure, partial read, or
			// oversized response has no trustworthy byte/item total. Retain the
			// full durable reservation so restart cannot reset the budget.
			return err
		}
		if statusCode == http.StatusTooManyRequests {
			if err := settle(0, int64(len(data))); err != nil {
				return err
			}
			delaySeconds, parseErr := strconv.Atoi(strings.TrimSpace(responseHeader.Get("Retry-After")))
			delay := time.Duration(delaySeconds) * time.Second
			if parseErr != nil || delay <= 0 || delay > client.budgets.MaximumRetryAfter || client.budget.ReserveRetry(ctx, client.now().UTC()) != nil {
				return ErrSlackRateLimited
			}
			if err := client.sleep(ctx, delay); err != nil {
				return err
			}
			continue
		}
		if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
			if err := settle(0, int64(len(data))); err != nil {
				return err
			}
			client.markTerminal(ErrSlackRevoked)
			return ErrSlackRevoked
		}
		if statusCode != http.StatusOK {
			if err := settle(0, int64(len(data))); err != nil {
				return err
			}
			return fmt.Errorf("Slack Web API status %d", statusCode)
		}
		if err := validateSlackScopes(responseHeader.Get("X-OAuth-Scopes"), method); err != nil {
			if settleErr := settle(0, int64(len(data))); settleErr != nil {
				return settleErr
			}
			return err
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(target); err != nil {
			if settleErr := settle(0, int64(len(data))); settleErr != nil {
				return settleErr
			}
			return err
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			if settleErr := settle(0, int64(len(data))); settleErr != nil {
				return settleErr
			}
			return errors.New("Slack response has trailing data")
		}
		envelopeData, _ := json.Marshal(target)
		var envelope slackEnvelope
		_ = json.Unmarshal(envelopeData, &envelope)
		if !envelope.OK {
			if err := settle(0, int64(len(data))); err != nil {
				return err
			}
			switch envelope.Error {
			case "invalid_auth", "token_revoked", "account_inactive":
				client.markTerminal(ErrSlackRevoked)
				return ErrSlackRevoked
			case "not_in_channel", "channel_not_found", "missing_scope":
				return ErrSlackChannelAccess
			default:
				return fmt.Errorf("Slack Web API rejected %s", method)
			}
		}
		items := 0
		if itemCount != nil {
			items = itemCount()
		}
		if err := settle(items, int64(len(data))); err != nil {
			return err
		}
		return nil
	}
}

func zeroSlackSecret(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func validateSlackScopes(header, method string) error {
	if strings.TrimSpace(header) == "" {
		return errors.New("Slack scope evidence missing")
	}
	allowed := map[string]bool{"channels:history": true, "groups:history": true, "im:history": true, "mpim:history": true, "files:read": true}
	history := false
	for _, scope := range strings.Split(header, ",") {
		scope = strings.TrimSpace(scope)
		if !allowed[scope] {
			return errors.New("Slack connection contains a non-read-only or unsupported scope")
		}
		if strings.HasSuffix(scope, ":history") {
			history = true
		}
	}
	if method != "auth.test" && !history {
		return ErrSlackChannelAccess
	}
	return nil
}

func readSlackBody(reader io.Reader, maximum int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, ErrSlackBudgetExceeded
	}
	return data, nil
}

func (client *SlackHTTPClient) markTerminal(err error) {
	client.mu.Lock()
	client.terminal = err
	client.mu.Unlock()
}

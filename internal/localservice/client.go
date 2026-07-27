package localservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/synergyai-os/Mindline/internal/agentstate"
	"github.com/synergyai-os/Mindline/internal/personalmemory"
)

const maximumResponseBytes = 64 << 20

type Client struct {
	socketPath string
	client     *http.Client
}

func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 2 * time.Second}
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{
		socketPath: socketPath,
		client:     &http.Client{Transport: transport, Timeout: 2 * time.Minute},
	}
}

func (client *Client) Status(ctx context.Context) (Status, error) {
	var status Status
	err := client.do(ctx, http.MethodGet, "/v1/status", nil, &status)
	return status, err
}

func (client *Client) Capabilities(ctx context.Context) (Capabilities, error) {
	var capabilities Capabilities
	err := client.do(ctx, http.MethodGet, "/v1/capabilities", nil, &capabilities)
	return capabilities, err
}

func (client *Client) Search(ctx context.Context, input SearchInput) (personalmemory.ContextPacket, error) {
	var packet personalmemory.ContextPacket
	err := client.do(ctx, http.MethodPost, "/v1/search", input, &packet)
	return packet, err
}

func (client *Client) SearchCompact(ctx context.Context, input SearchInput) (personalmemory.CompactContextPacket, error) {
	var packet personalmemory.CompactContextPacket
	err := client.do(ctx, http.MethodPost, "/v1/search/compact", input, &packet)
	return packet, err
}

func (client *Client) Get(ctx context.Context, recordID string) (personalmemory.HydratedCapture, error) {
	var capture personalmemory.HydratedCapture
	err := client.do(ctx, http.MethodGet, "/v1/captures/"+url.PathEscape(recordID), nil, &capture)
	return capture, err
}

func (client *Client) ListLenses(ctx context.Context) ([]agentstate.Lens, error) {
	var lenses []agentstate.Lens
	err := client.do(ctx, http.MethodGet, "/v1/lenses", nil, &lenses)
	return lenses, err
}

func (client *Client) PutLens(ctx context.Context, lens agentstate.Lens) (agentstate.Lens, error) {
	var saved agentstate.Lens
	err := client.do(ctx, http.MethodPut, "/v1/lenses/"+url.PathEscape(lens.ID), lens, &saved)
	return saved, err
}

func (client *Client) DeleteLens(ctx context.Context, id string) (DeleteResult, error) {
	var result DeleteResult
	err := client.do(ctx, http.MethodDelete, "/v1/lenses/"+url.PathEscape(id), nil, &result)
	return result, err
}

func (client *Client) ApplyJudgment(ctx context.Context, input agentstate.JudgmentRequest) (agentstate.Judgment, error) {
	var judgment agentstate.Judgment
	err := client.do(ctx, http.MethodPost, "/v1/judgments", input, &judgment)
	return judgment, err
}

func (client *Client) do(ctx context.Context, method, endpoint string, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return errors.New("encode local agent request")
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://mindline.local"+endpoint, body)
	if err != nil {
		return errors.New("create local agent request")
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.client.Do(request)
	if err != nil {
		return errors.New("local agent service is unavailable")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil || len(data) > maximumResponseBytes {
		return errors.New("read local agent response")
	}
	var envelope struct {
		SchemaVersion string          `json:"schema_version"`
		Data          json.RawMessage `json:"data"`
		Error         string          `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.SchemaVersion != APISchemaVersion {
		return errors.New("invalid local agent response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if strings.TrimSpace(envelope.Error) == "" {
			return errors.New("local agent request failed")
		}
		return errors.New(envelope.Error)
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return errors.New("decode local agent response")
	}
	return nil
}

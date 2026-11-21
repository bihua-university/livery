package clashapi

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/bihua-university/livery/internal/model"
)

// ErrUnsupported indicates that the connected core's Clash API does not implement the requested endpoint.
var ErrUnsupported = errors.New("clash api endpoint unsupported")

// Client speaks to sing-box/mihomo Clash API HTTP endpoints.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
	stream  *http.Client
}

// New creates a Clash API client targeting host:port with an optional bearer secret.
func New(host string, port int, secret string) *Client {
	if host == "" {
		host = model.DefaultClashAPIHost
	}
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	return &Client{
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		secret:  secret,
		http: &http.Client{
			Timeout: 8 * time.Second,
			Transport: &http.Transport{
				Proxy:       nil,
				DialContext: dialer.DialContext,
			},
		},
		stream: &http.Client{
			Timeout: 0,
			Transport: &http.Transport{
				Proxy:                 nil,
				DialContext:           dialer.DialContext,
				ResponseHeaderTimeout: 5 * time.Second,
			},
		},
	}
}

// CloseIdleConnections closes any idle keep-alive sockets held by the Clash API HTTP transports.
func (c *Client) CloseIdleConnections() {
	if c == nil {
		return
	}
	if c.http != nil {
		c.http.CloseIdleConnections()
	}
	if c.stream != nil {
		c.stream.CloseIdleConnections()
	}
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	return c.request(ctx, c.http, method, path, body)
}

func (c *Client) request(ctx context.Context, transport *http.Client, method, path string, body any) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	return transport.Do(req)
}

func (c *Client) doStream(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	return c.stream.Do(req)
}

func decodeJSON(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("clash api HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Version returns the version string reported by Clash API (without leading "v").
func (c *Client) Version(ctx context.Context) (string, error) {
	resp, err := c.do(ctx, http.MethodGet, "/version", nil)
	if err != nil {
		return "", err
	}
	var res struct {
		Version string `json:"version"`
	}
	if err := decodeJSON(resp, &res); err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(res.Version), "v"), nil
}

// GetMode returns the current proxy mode ("rule", "global", "direct").
func (c *Client) GetMode(ctx context.Context) (model.ProxyMode, error) {
	resp, err := c.do(ctx, http.MethodGet, "/configs", nil)
	if err != nil {
		return "", err
	}
	var res struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSON(resp, &res); err != nil {
		return "", err
	}
	return model.ProxyMode(strings.ToLower(res.Mode)), nil
}

// ProxyURL returns the inbound address on the controller's host. Mihomo
// reports active listeners, including ports preserved from imported profiles.
func (c *Client) ProxyURL(ctx context.Context, coreType model.CoreType, configuredPort int) (*url.URL, error) {
	endpoint, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	scheme, port := "http", configuredPort
	if model.NormalizeCoreType(coreType) == model.CoreMihomo {
		resp, err := c.do(ctx, http.MethodGet, "/configs", nil)
		if err != nil {
			return nil, err
		}
		var config struct {
			Mixed *int `json:"mixed-port"`
			HTTP  *int `json:"port"`
			SOCKS *int `json:"socks-port"`
		}
		if err := decodeJSON(resp, &config); err != nil {
			return nil, err
		}
		switch {
		case config.Mixed != nil && *config.Mixed > 0:
			port = *config.Mixed
		case config.HTTP != nil && *config.HTTP > 0:
			port = *config.HTTP
		case config.SOCKS != nil && *config.SOCKS > 0:
			scheme, port = "socks5", *config.SOCKS
		case config.Mixed != nil || config.HTTP != nil || config.SOCKS != nil:
			port = 0
		}
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("no active HTTP, mixed or SOCKS proxy port")
	}
	return &url.URL{Scheme: scheme, Host: net.JoinHostPort(endpoint.Hostname(), fmt.Sprint(port))}, nil
}

// SetMode changes the active proxy mode via PATCH /configs.
func (c *Client) SetMode(ctx context.Context, mode model.ProxyMode) error {
	return c.PatchConfig(ctx, map[string]any{"mode": string(mode)})
}

func (c *Client) PatchConfig(ctx context.Context, fields map[string]any) error {
	resp, err := c.do(ctx, http.MethodPatch, "/configs", fields)
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}

// PatchInbound checks the listener after PATCH: Mihomo can return 204 even
// when a replacement listener could not bind its port.
func (c *Client) PatchInbound(ctx context.Context, field string, value any) error {
	read := func() (json.RawMessage, error) {
		resp, err := c.do(ctx, http.MethodGet, "/configs", nil)
		if err != nil {
			return nil, err
		}
		var current map[string]json.RawMessage
		if err := decodeJSON(resp, &current); err != nil {
			return nil, err
		}
		if len(current[field]) == 0 {
			return nil, fmt.Errorf("controller does not report %s", field)
		}
		return current[field], nil
	}
	previous, err := read()
	if err != nil {
		return err
	}
	want, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if bytes.Equal(previous, want) {
		return nil
	}
	if err := c.PatchConfig(ctx, map[string]any{field: value}); err != nil {
		return err
	}
	current, err := read()
	if err != nil {
		return err
	}
	if bytes.Equal(current, want) {
		return nil
	}
	// A failed bind can leave the previous listener closed.
	if !bytes.Equal(current, previous) {
		if err := c.PatchConfig(ctx, map[string]any{field: previous}); err != nil {
			return fmt.Errorf("restore %s after failed update: %w", field, err)
		}
		restored, err := read()
		if err != nil {
			return err
		}
		if !bytes.Equal(restored, previous) {
			return fmt.Errorf("controller failed to restore %s=%s", field, previous)
		}
	}
	return fmt.Errorf("controller did not apply %s=%s", field, want)
}

// ReloadConfig applies a compiled profile within the existing Mihomo process.
func (c *Client) ReloadConfig(ctx context.Context, payload []byte) error {
	// Reload may wait for rule and proxy providers before acknowledging.
	httpClient := &http.Client{Transport: c.http.Transport, Timeout: 90 * time.Second}
	resp, err := c.request(ctx, httpClient, http.MethodPut, "/configs?force=true", map[string]string{"payload": string(payload)})
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}

func (c *Client) FlushDNSCaches(ctx context.Context) error {
	var errs []error
	for _, cache := range []string{"dns", "fakeip"} {
		resp, err := c.do(ctx, http.MethodPost, "/cache/"+cache+"/flush", nil)
		if err == nil {
			err = decodeJSON(resp, nil)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", cache, err))
		}
	}
	return errors.Join(errs...)
}

// UpdateGeo triggers Mihomo's POST /configs/geo endpoint to update and reload GeoIP/GeoSite databases.
func (c *Client) UpdateGeo(ctx context.Context) error {
	callCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	resp, err := c.do(callCtx, http.MethodPost, "/configs/geo", nil)
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}

type rawProxy struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Now     string   `json:"now"`
	All     []string `json:"all"`
	UDP     bool     `json:"udp"`
	Alive   *bool    `json:"alive"`
	History []struct {
		Delay int `json:"delay"`
	} `json:"history"`
}

// GetProxies returns all proxy groups and individual outbound nodes.
func (c *Client) GetProxies(ctx context.Context) ([]model.ProxyGroup, map[string]model.ProxyNode, error) {
	resp, err := c.do(ctx, http.MethodGet, "/proxies", nil)
	if err != nil {
		return nil, nil, err
	}
	var res struct {
		Proxies map[string]rawProxy `json:"proxies"`
	}
	if err := decodeJSON(resp, &res); err != nil {
		return nil, nil, err
	}

	nodes := make(map[string]model.ProxyNode, len(res.Proxies))
	var groups []model.ProxyGroup

	for name, raw := range res.Proxies {
		delay := -1
		if len(raw.History) > 0 {
			delay = raw.History[len(raw.History)-1].Delay
		}
		if raw.Alive != nil && !*raw.Alive {
			delay = 0
		}
		nodeName := raw.Name
		if nodeName == "" {
			nodeName = name
		}
		nodes[nodeName] = model.ProxyNode{
			Name:  nodeName,
			Type:  raw.Type,
			Delay: delay,
			UDP:   raw.UDP,
		}
		if len(raw.All) > 0 && !strings.EqualFold(nodeName, "DIRECT") && !strings.EqualFold(nodeName, "REJECT") {
			groups = append(groups, model.ProxyGroup{
				Name: nodeName,
				Type: raw.Type,
				Now:  raw.Now,
				All:  raw.All,
			})
		}
	}

	// Preserve config declaration order via GLOBAL.All when available.
	var orderIdx map[string]int
	if global, ok := res.Proxies["GLOBAL"]; ok && len(global.All) > 0 {
		orderIdx = make(map[string]int, len(global.All))
		for idx, member := range global.All {
			orderIdx[member] = idx
		}
	}

	globalRank := func(name string) int {
		if name == "GLOBAL" {
			return 1
		}
		return 0
	}
	orderRank := func(name string) int {
		if idx, ok := orderIdx[name]; ok {
			return idx
		}
		return len(orderIdx)
	}
	selectorRank := func(typ string) int {
		if strings.EqualFold(typ, "Selector") {
			return 0
		}
		return 1
	}

	// Keep GLOBAL last and sort other groups by config order, then Selector first, then name.
	slices.SortFunc(groups, func(a, b model.ProxyGroup) int {
		return cmp.Or(
			cmp.Compare(globalRank(a.Name), globalRank(b.Name)),
			cmp.Compare(orderRank(a.Name), orderRank(b.Name)),
			cmp.Compare(selectorRank(a.Type), selectorRank(b.Type)),
			cmp.Compare(a.Name, b.Name),
		)
	})

	return groups, nodes, nil
}

// SelectProxy sets the active outbound for a Selector group.
func (c *Client) SelectProxy(ctx context.Context, group, target string) error {
	path := "/proxies/" + url.PathEscape(group)
	resp, err := c.do(ctx, http.MethodPut, path, map[string]string{"name": target})
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}

// A latency test can run longer than the client's ordinary REST timeout.
func (c *Client) delayRequest(ctx context.Context, path string, timeout time.Duration) (*http.Response, error) {
	client := &http.Client{Transport: c.http.Transport, Timeout: timeout}
	return c.request(ctx, client, http.MethodGet, path, nil)
}

// TestDelay measures latency (ms) of a single proxy node.
func (c *Client) TestDelay(ctx context.Context, proxyName, testURL string, timeoutMs int) (int, error) {
	if testURL == "" {
		testURL = model.DefaultDelayTestURL
	}
	if timeoutMs <= 0 {
		timeoutMs = model.DefaultDelayTestTimeoutMs
	}
	q := url.Values{}
	q.Set("url", testURL)
	q.Set("timeout", fmt.Sprintf("%d", timeoutMs))
	path := "/proxies/" + url.PathEscape(proxyName) + "/delay?" + q.Encode()

	resp, err := c.delayRequest(ctx, path, time.Duration(timeoutMs)*time.Millisecond+2*time.Second)
	if err != nil {
		return 0, err
	}
	var res struct {
		Delay int `json:"delay"`
	}
	if err := decodeJSON(resp, &res); err != nil {
		return 0, err
	}
	return res.Delay, nil
}

// TestGroupDelay tests all members of a proxy group concurrently via Clash API.
func (c *Client) TestGroupDelay(ctx context.Context, groupName, testURL string, timeoutMs int) (map[string]int, error) {
	if testURL == "" {
		testURL = model.DefaultDelayTestURL
	}
	if timeoutMs <= 0 {
		timeoutMs = model.DefaultDelayTestTimeoutMs
	}
	q := url.Values{}
	q.Set("url", testURL)
	q.Set("timeout", fmt.Sprintf("%d", timeoutMs))
	path := "/group/" + url.PathEscape(groupName) + "/delay?" + q.Encode()

	resp, err := c.delayRequest(ctx, path, time.Duration(timeoutMs)*time.Millisecond+3*time.Second)
	if err != nil {
		return nil, err
	}
	var res map[string]int
	if err := decodeJSON(resp, &res); err != nil {
		return nil, err
	}
	return res, nil
}

// GetConnections fetches active connections and aggregate totals.
func (c *Client) GetConnections(ctx context.Context) (model.ConnectionsSnapshot, error) {
	resp, err := c.do(ctx, http.MethodGet, "/connections", nil)
	if err != nil {
		return model.ConnectionsSnapshot{}, err
	}
	var snap model.ConnectionsSnapshot
	if err := decodeJSON(resp, &snap); err != nil {
		return model.ConnectionsSnapshot{}, err
	}
	return snap, nil
}

// CloseConnection terminates an active connection by ID.
func (c *Client) CloseConnection(ctx context.Context, id string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/connections/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}

// CloseAllConnections terminates every active connection.
func (c *Client) CloseAllConnections(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodDelete, "/connections", nil)
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
}

// GetRules fetches the active routing rules from Clash API.
func (c *Client) GetRules(ctx context.Context) ([]model.RuleEntry, error) {
	resp, err := c.do(ctx, http.MethodGet, "/rules", nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Rules []model.RuleEntry `json:"rules"`
	}
	if err := decodeJSON(resp, &res); err != nil {
		return nil, err
	}
	return res.Rules, nil
}

func streamNDJSON[T any](ctx context.Context, c *Client, name, path string, maxBuf int, checkUnsupported bool, onItem func(T)) error {
	resp, err := c.doStream(ctx, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if checkUnsupported && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented) {
		return ErrUnsupported
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s stream HTTP %d", name, resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	if maxBuf > 0 {
		buf := make([]byte, 0, min(64*1024, maxBuf))
		scanner.Buffer(buf, maxBuf)
	}
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var item T
		if json.Unmarshal(line, &item) == nil {
			onItem(item)
		}
	}
	return scanner.Err()
}

// StreamTraffic reads the newline-delimited JSON stream from /traffic until ctx is canceled.
func (c *Client) StreamTraffic(ctx context.Context, onSample func(model.TrafficSample)) error {
	type trafficPayload struct {
		Up   int64 `json:"up"`
		Down int64 `json:"down"`
	}
	return streamNDJSON(ctx, c, "traffic", "/traffic", 0, false, func(payload trafficPayload) {
		onSample(model.TrafficSample{
			Timestamp: time.Now(),
			Up:        payload.Up,
			Down:      payload.Down,
		})
	})
}

// StreamLogs reads the newline-delimited JSON stream from /logs until ctx is canceled.
func (c *Client) StreamLogs(ctx context.Context, level string, onLog func(model.LogEntry)) error {
	if level == "" {
		level = "info"
	}
	type logPayload struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
	}
	return streamNDJSON(ctx, c, "logs", "/logs?level="+url.QueryEscape(level), 1024*1024, false, func(payload logPayload) {
		if payload.Payload != "" {
			onLog(model.LogEntry{
				Time:    time.Now(),
				Level:   strings.ToLower(payload.Type),
				Source:  "sing-box",
				Message: payload.Payload,
			})
		}
	})
}

// StreamMemory reads the newline-delimited JSON stream from /memory until ctx is canceled.
// Mihomo reports core memory via /memory ({"inuse":..., "oslimit":...}) rather than /connections.
func (c *Client) StreamMemory(ctx context.Context, onMemory func(int64)) error {
	type memoryPayload struct {
		Inuse     int64 `json:"inuse"`
		Footprint int64 `json:"footprint"`
	}
	return streamNDJSON(ctx, c, "memory", "/memory", 0, true, func(payload memoryPayload) {
		mem := payload.Inuse
		if mem <= 0 {
			mem = payload.Footprint
		}
		if mem > 0 {
			onMemory(mem)
		}
	})
}

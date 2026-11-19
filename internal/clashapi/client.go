package clashapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
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
		host = "127.0.0.1"
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
	return c.http.Do(req)
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

// SetMode changes the active proxy mode via PATCH /configs.
func (c *Client) SetMode(ctx context.Context, mode model.ProxyMode) error {
	resp, err := c.do(ctx, http.MethodPatch, "/configs", map[string]string{"mode": string(mode)})
	if err != nil {
		return err
	}
	return decodeJSON(resp, nil)
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

	// Keep GLOBAL last and sort other groups by config order, then Selector first, then name.
	sort.Slice(groups, func(i, j int) bool {
		gi, gj := groups[i], groups[j]
		if gi.Name == "GLOBAL" || gj.Name == "GLOBAL" {
			return gj.Name == "GLOBAL" && gi.Name != "GLOBAL"
		}
		oi, hasI := orderIdx[gi.Name]
		oj, hasJ := orderIdx[gj.Name]
		if hasI && hasJ && oi != oj {
			return oi < oj
		}
		if hasI != hasJ {
			return hasI
		}
		si := strings.EqualFold(gi.Type, "Selector")
		sj := strings.EqualFold(gj.Type, "Selector")
		if si != sj {
			return si
		}
		return gi.Name < gj.Name
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

// TestDelay measures latency (ms) of a single proxy node.
func (c *Client) TestDelay(ctx context.Context, proxyName, testURL string, timeoutMs int) (int, error) {
	if testURL == "" {
		testURL = "http://www.gstatic.com/generate_204"
	}
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	q := url.Values{}
	q.Set("url", testURL)
	q.Set("timeout", fmt.Sprintf("%d", timeoutMs))
	path := "/proxies/" + url.PathEscape(proxyName) + "/delay?" + q.Encode()

	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs+2000)*time.Millisecond)
	defer cancel()

	resp, err := c.do(callCtx, http.MethodGet, path, nil)
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
		testURL = "http://www.gstatic.com/generate_204"
	}
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	q := url.Values{}
	q.Set("url", testURL)
	q.Set("timeout", fmt.Sprintf("%d", timeoutMs))
	path := "/group/" + url.PathEscape(groupName) + "/delay?" + q.Encode()

	callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs+3000)*time.Millisecond)
	defer cancel()

	resp, err := c.do(callCtx, http.MethodGet, path, nil)
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

// StreamTraffic reads the newline-delimited JSON stream from /traffic until ctx is canceled.
func (c *Client) StreamTraffic(ctx context.Context, onSample func(model.TrafficSample)) error {
	resp, err := c.doStream(ctx, "/traffic")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("traffic stream HTTP %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var payload struct {
			Up   int64 `json:"up"`
			Down int64 `json:"down"`
		}
		if json.Unmarshal(line, &payload) == nil {
			onSample(model.TrafficSample{
				Timestamp: time.Now(),
				Up:        payload.Up,
				Down:      payload.Down,
			})
		}
	}
	return scanner.Err()
}

// StreamLogs reads the newline-delimited JSON stream from /logs until ctx is canceled.
func (c *Client) StreamLogs(ctx context.Context, level string, onLog func(model.LogEntry)) error {
	if level == "" {
		level = "info"
	}
	resp, err := c.doStream(ctx, "/logs?level="+url.QueryEscape(level))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("logs stream HTTP %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var payload struct {
			Type    string `json:"type"`
			Payload string `json:"payload"`
		}
		if json.Unmarshal(line, &payload) == nil && payload.Payload != "" {
			onLog(model.LogEntry{
				Time:    time.Now(),
				Level:   strings.ToLower(payload.Type),
				Source:  "sing-box",
				Message: payload.Payload,
			})
		}
	}
	return scanner.Err()
}

// StreamMemory reads the newline-delimited JSON stream from /memory until ctx is canceled.
// Mihomo reports core memory via /memory ({"inuse":..., "oslimit":...}) rather than /connections.
func (c *Client) StreamMemory(ctx context.Context, onMemory func(int64)) error {
	resp, err := c.doStream(ctx, "/memory")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented {
		return ErrUnsupported
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("memory stream HTTP %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var payload struct {
			Inuse     int64 `json:"inuse"`
			Footprint int64 `json:"footprint"`
		}
		if json.Unmarshal(line, &payload) == nil {
			mem := payload.Inuse
			if mem <= 0 {
				mem = payload.Footprint
			}
			if mem > 0 {
				onMemory(mem)
			}
		}
	}
	return scanner.Err()
}


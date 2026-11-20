// Package diagnostics checks the current outbound route through the core's HTTP proxy.
package diagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

type IPQuality struct {
	IP       string                         `json:"ip"`
	Location struct{ Country, City string } `json:"location"`
	Risk     struct {
		Score      *int `json:"risk_score"`
		VPN        bool `json:"is_vpn"`
		Tor        bool `json:"is_tor"`
		Proxy      bool `json:"is_proxy"`
		Datacenter bool `json:"is_datacenter"`
	} `json:"risk"`
}

type ServiceResult struct {
	Name    string
	Status  int
	Elapsed time.Duration
}

type Report struct {
	IP        string
	Quality   IPQuality
	IPError   error
	Services  []ServiceResult
	CheckedAt time.Time
}

var services = [...]struct{ name, address string }{
	{"Google", "https://www.google.com/generate_204"},
	{"GitHub", "https://github.com/"},
	{"YouTube", "https://www.youtube.com/generate_204"},
	{"ChatGPT", "https://chatgpt.com/"},
}

func (c *Checker) Check(ctx context.Context, proxy *url.URL) Report {
	if proxy == nil || proxy.Host == "" {
		return Report{CheckedAt: time.Now(), IPError: fmt.Errorf("proxy endpoint unavailable")}
	}
	client := &http.Client{
		Transport:     &http.Transport{Proxy: http.ProxyURL(proxy)},
		Timeout:       6 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	return c.check(ctx, client)
}

func (c *Checker) check(ctx context.Context, client *http.Client) Report {
	report := Report{Services: make([]ServiceResult, len(services))}
	var workers sync.WaitGroup
	for i, service := range services {
		workers.Go(func() {
			start := time.Now()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, service.address, nil)
			result := ServiceResult{Name: service.name}
			if resp, err := client.Do(req); err == nil {
				result.Status = resp.StatusCode
				resp.Body.Close()
			}
			result.Elapsed = time.Since(start)
			report.Services[i] = result
		})
	}
	report.IP, report.Quality, report.IPError = c.lookup(ctx, client)
	workers.Wait()
	report.CheckedAt = time.Now()
	return report
}

func get(ctx context.Context, client *http.Client, address string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", req.URL.Host, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
}

func (c *Checker) lookup(ctx context.Context, client *http.Client) (string, IPQuality, error) {
	body, err := get(ctx, client, "https://api64.ipify.org/")
	if err != nil {
		return "", IPQuality{}, err
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(string(body)))
	if err != nil || addr.Zone() != "" {
		return "", IPQuality{}, fmt.Errorf("invalid IP from ipify")
	}
	ip := addr.Unmap().String()
	if cached, ok := c.cached(ip); ok {
		return ip, cached, nil
	}
	body, err = get(ctx, client, "https://api.ipquery.io/"+ip+"?format=json")
	if err != nil {
		return ip, IPQuality{}, err
	}
	var quality IPQuality
	if err := json.Unmarshal(body, &quality); err != nil {
		return ip, IPQuality{}, err
	}
	got, err := netip.ParseAddr(quality.IP)
	if err != nil || got.Unmap() != addr.Unmap() {
		return ip, IPQuality{}, fmt.Errorf("IPQuery returned a different IP")
	}
	if score := quality.Risk.Score; score != nil && *score >= 0 && *score <= 100 {
		c.store(ip, quality)
	} else {
		quality.Risk.Score = nil
	}
	return ip, quality, nil
}

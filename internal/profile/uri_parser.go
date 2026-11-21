package profile

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

type parsedBaseURI struct {
	host     string
	port     int
	tag      string
	username string
	password string
	query    url.Values
	alpn     []string
}

func parseBaseURI(raw string) (parsedBaseURI, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return parsedBaseURI{}, false
	}
	port, _ := strconv.Atoi(u.Port())
	if port <= 0 {
		port = 443
	}
	q := u.Query()
	tag := u.Fragment
	if tag == "" {
		tag = u.Hostname()
	}
	var alpn []string
	if a := q.Get("alpn"); a != "" {
		alpn = strings.Split(a, ",")
	}
	pw, _ := u.User.Password()
	return parsedBaseURI{
		host:     u.Hostname(),
		port:     port,
		tag:      tag,
		username: u.User.Username(),
		password: pw,
		query:    q,
		alpn:     alpn,
	}, true
}

func buildQueryTransport(q url.Values) map[string]any {
	switch strings.ToLower(q.Get("type")) {
	case "ws":
		tr := map[string]any{"type": "ws"}
		if p := q.Get("path"); p != "" {
			tr["path"] = p
		}
		if h := q.Get("host"); h != "" {
			tr["headers"] = map[string]any{"Host": h}
		}
		return tr
	case "grpc":
		tr := map[string]any{"type": "grpc"}
		if sn := q.Get("serviceName"); sn != "" {
			tr["service_name"] = sn
		}
		return tr
	}
	return nil
}

func parseShareURI(raw string) (map[string]any, bool) {
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "vmess://"):
		return parseVMessURI(raw[len("vmess://"):])
	case strings.HasPrefix(lower, "vless://"):
		return parseVLESSURI(raw)
	case strings.HasPrefix(lower, "trojan://"):
		return parseTrojanURI(raw)
	case strings.HasPrefix(lower, "ss://"):
		return parseSSURI(raw)
	case strings.HasPrefix(lower, "hysteria2://"), strings.HasPrefix(lower, "hy2://"):
		return parseHysteria2URI(raw)
	case strings.HasPrefix(lower, "tuic://"):
		return parseTUICURI(raw)
	}
	return nil, false
}

func parseVMessURI(payload string) (map[string]any, bool) {
	decoded, ok := decodeBase64Flexible(payload)
	if !ok {
		return nil, false
	}
	var v struct {
		PS   string `json:"ps"`
		Add  string `json:"add"`
		Port any    `json:"port"`
		ID   string `json:"id"`
		Aid  any    `json:"aid"`
		Scy  string `json:"scy"`
		Net  string `json:"net"`
		Type string `json:"type"`
		Host string `json:"host"`
		Path string `json:"path"`
		TLS  string `json:"tls"`
		SNI  string `json:"sni"`
		Fp   string `json:"fp"`
	}
	if err := json.Unmarshal([]byte(decoded), &v); err != nil || v.Add == "" || v.ID == "" {
		return nil, false
	}
	port := intVal(v.Port)
	if port <= 0 {
		return nil, false
	}
	tag := v.PS
	if tag == "" {
		tag = v.Add
	}
	scy := v.Scy
	if scy == "" {
		scy = "auto"
	}
	ob := map[string]any{
		"type":        "vmess",
		"tag":         tag,
		"server":      v.Add,
		"server_port": port,
		"uuid":        v.ID,
		"security":    scy,
	}
	if aid := intVal(v.Aid); aid > 0 {
		ob["alter_id"] = aid
	}
	if strings.EqualFold(v.TLS, "tls") {
		ob["tls"] = buildSingBoxTLS(v.SNI, v.Host, false, nil, v.Fp, nil)
	}
	if strings.EqualFold(v.Net, "ws") {
		tr := map[string]any{"type": "ws"}
		if v.Path != "" {
			tr["path"] = v.Path
		}
		if v.Host != "" {
			tr["headers"] = map[string]any{"Host": v.Host}
		}
		ob["transport"] = tr
	} else if strings.EqualFold(v.Net, "grpc") {
		tr := map[string]any{"type": "grpc"}
		if v.Path != "" {
			tr["service_name"] = v.Path
		}
		ob["transport"] = tr
	}
	return ob, true
}

func parseVLESSURI(raw string) (map[string]any, bool) {
	b, ok := parseBaseURI(raw)
	if !ok {
		return nil, false
	}
	ob := map[string]any{
		"type":        "vless",
		"tag":         b.tag,
		"server":      b.host,
		"server_port": b.port,
		"uuid":        b.username,
	}
	if flow := b.query.Get("flow"); flow != "" {
		ob["flow"] = flow
	}
	sec := strings.ToLower(b.query.Get("security"))
	if sec == "tls" || sec == "reality" {
		var reality map[string]any
		if sec == "reality" {
			reality = map[string]any{
				"public-key": b.query.Get("pbk"),
				"short-id":   b.query.Get("sid"),
			}
		}
		ob["tls"] = buildSingBoxTLS(b.query.Get("sni"), "", b.query.Get("allowInsecure") == "1", b.alpn, b.query.Get("fp"), reality)
	}
	if tr := buildQueryTransport(b.query); tr != nil {
		ob["transport"] = tr
	}
	return ob, true
}

func parseTrojanURI(raw string) (map[string]any, bool) {
	b, ok := parseBaseURI(raw)
	if !ok {
		return nil, false
	}
	ob := map[string]any{
		"type":        "trojan",
		"tag":         b.tag,
		"server":      b.host,
		"server_port": b.port,
		"password":    b.username,
		"tls":         buildSingBoxTLS(b.query.Get("sni"), "", b.query.Get("allowInsecure") == "1", b.alpn, b.query.Get("fp"), nil),
	}
	if tr := buildQueryTransport(b.query); tr != nil {
		ob["transport"] = tr
	}
	return ob, true
}

func parseSSURI(raw string) (map[string]any, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, false
	}
	tag := u.Fragment
	host := u.Hostname()
	portStr := u.Port()
	var method, password string

	if u.User != nil && u.User.Username() != "" {
		if pw, hasPW := u.User.Password(); hasPW {
			method = u.User.Username()
			password = pw
		} else if dec, ok := decodeBase64Flexible(u.User.Username()); ok {
			if m, p, found := strings.Cut(dec, ":"); found {
				method, password = m, p
			}
		}
	} else {
		// SIP002 legacy base64(method:password@host:port)
		body := strings.TrimPrefix(raw, "ss://")
		if idx := strings.Index(body, "#"); idx >= 0 {
			tag, _ = url.QueryUnescape(body[idx+1:])
			body = body[:idx]
		}
		if dec, ok := decodeBase64Flexible(body); ok {
			if creds, hostPort, found := strings.Cut(dec, "@"); found {
				method, password, _ = strings.Cut(creds, ":")
				if h, p, ok2 := strings.Cut(hostPort, ":"); ok2 {
					host, portStr = h, p
				}
			}
		}
	}

	port, _ := strconv.Atoi(portStr)
	if host == "" || port <= 0 || method == "" {
		return nil, false
	}
	if tag == "" {
		tag = host
	}
	return map[string]any{
		"type":        "shadowsocks",
		"tag":         tag,
		"server":      host,
		"server_port": port,
		"method":      method,
		"password":    password,
	}, true
}

func parseHysteria2URI(raw string) (map[string]any, bool) {
	normalized := raw
	if strings.HasPrefix(strings.ToLower(raw), "hy2://") {
		normalized = "hysteria2://" + raw[len("hy2://"):]
	}
	b, ok := parseBaseURI(normalized)
	if !ok {
		return nil, false
	}
	ob := map[string]any{
		"type":        "hysteria2",
		"tag":         b.tag,
		"server":      b.host,
		"server_port": b.port,
		"password":    b.username,
		"tls":         buildSingBoxTLS(b.query.Get("sni"), "", b.query.Get("insecure") == "1", nil, "", nil),
	}
	if obfs := b.query.Get("obfs"); obfs != "" {
		ob["obfs"] = map[string]any{
			"type":     obfs,
			"password": b.query.Get("obfs-password"),
		}
	}
	return ob, true
}

func parseTUICURI(raw string) (map[string]any, bool) {
	b, ok := parseBaseURI(raw)
	if !ok {
		return nil, false
	}
	ob := map[string]any{
		"type":        "tuic",
		"tag":         b.tag,
		"server":      b.host,
		"server_port": b.port,
		"uuid":        b.username,
		"password":    b.password,
		"tls":         buildSingBoxTLS(b.query.Get("sni"), "", b.query.Get("allow_insecure") == "1", b.alpn, "", nil),
	}
	if cc := b.query.Get("congestion_control"); cc != "" {
		ob["congestion_control"] = cc
	}
	return ob, true
}

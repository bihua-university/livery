package profile

import (
	"bytes"
	"encoding/json"
	"errors"

	"gopkg.in/yaml.v3"

	"github.com/bihua-university/livery/internal/model"
)

type payloadKind string

const (
	payloadSingBox payloadKind = "sing-box"
	payloadClash   payloadKind = "clash"
	payloadURI     payloadKind = "uri"
)

type detectedPayload struct {
	kind payloadKind
	raw  []byte
	obj  map[string]any
	text string
}

func detectSubscription(raw []byte) (detectedPayload, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return detectedPayload{}, errors.New("empty subscription content")
	}

	if p, ok := detectDirectPayload(trimmed); ok {
		return p, nil
	}

	text := string(trimmed)
	if looksLikeURIList(text) {
		return detectedPayload{kind: payloadURI, text: text}, nil
	}

	if decoded, ok := decodeBase64Flexible(text); ok {
		if looksLikeURIList(decoded) {
			return detectedPayload{kind: payloadURI, text: decoded}, nil
		}
		if dt := bytes.TrimSpace([]byte(decoded)); len(dt) > 0 {
			if p, ok := detectDirectPayload(dt); ok {
				return p, nil
			}
		}
	}

	return detectedPayload{}, errors.New("unrecognized subscription format (expected Mihomo/Clash YAML, sing-box JSON, or Base64/URI links)")
}

func detectDirectPayload(trimmed []byte) (detectedPayload, bool) {
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var obj map[string]any
		if err := json.Unmarshal(trimmed, &obj); err == nil && obj != nil {
			return detectedPayload{kind: payloadSingBox, raw: trimmed, obj: obj}, true
		}
	}
	if isClashYAML(trimmed) {
		return detectedPayload{kind: payloadClash, raw: trimmed}, true
	}
	return detectedPayload{}, false
}

// DetectAndNormalizeProfile detects Mihomo/Clash YAML, sing-box JSON, or Base64/URI share links.
// Native Clash/Mihomo YAML and sing-box JSON are preserved in their native format; URI lists are
// converted to the preferredCore's native format.
func DetectAndNormalizeProfile(raw []byte, preferredCore model.CoreType) ([]byte, string, string, error) {
	p, err := detectSubscription(raw)
	if err != nil {
		return nil, "", "", err
	}
	switch p.kind {
	case payloadSingBox:
		out, err := json.MarshalIndent(p.obj, "", "  ")
		if err != nil {
			return nil, "", "", err
		}
		return append(out, '\n'), "sing-box", ".json", nil
	case payloadClash:
		return append(p.raw[:len(p.raw):len(p.raw)], '\n'), "mihomo", ".yaml", nil
	case payloadURI:
		if model.NormalizeCoreType(preferredCore) == model.CoreSingBox {
			out, err := convertURIList(p.text)
			if err != nil {
				return nil, "", "", err
			}
			return out, "sing-box", ".json", nil
		}
		out, err := convertURIListToMihomo(p.text)
		if err != nil {
			return nil, "", "", err
		}
		return out, "mihomo", ".yaml", nil
	}
	return nil, "", "", errors.New("unrecognized subscription format (expected Mihomo/Clash YAML, sing-box JSON, or Base64/URI links)")
}

// ConvertToMihomo detects Clash/Mihomo YAML, sing-box JSON, or Base64/URI share links
// and converts the input into Mihomo YAML.
func ConvertToMihomo(raw []byte) ([]byte, string, error) {
	p, err := detectSubscription(raw)
	if err != nil {
		return nil, "", err
	}
	switch p.kind {
	case payloadSingBox:
		out, err := ConvertSingBoxToMihomo(p.raw)
		if err != nil {
			return nil, "", err
		}
		return out, "sing-box", nil
	case payloadClash:
		return append(p.raw[:len(p.raw):len(p.raw)], '\n'), "clash", nil
	case payloadURI:
		out, err := convertURIListToMihomo(p.text)
		if err != nil {
			return nil, "", err
		}
		return out, "uri", nil
	}
	return nil, "", errors.New("unrecognized subscription format (expected Mihomo/Clash YAML, sing-box JSON, or Base64/URI links)")
}

// ConvertToSingBox detects native sing-box JSON, Clash/Mihomo YAML, or Base64/URI share links
// and converts the input into formatted sing-box JSON.
func ConvertToSingBox(raw []byte) ([]byte, string, error) {
	p, err := detectSubscription(raw)
	if err != nil {
		return nil, "", err
	}
	switch p.kind {
	case payloadSingBox:
		out, err := json.MarshalIndent(p.obj, "", "  ")
		if err != nil {
			return nil, "", err
		}
		return append(out, '\n'), "sing-box", nil
	case payloadClash:
		out, err := convertClashYAML(p.raw)
		if err != nil {
			return nil, "", err
		}
		return out, "clash", nil
	case payloadURI:
		out, err := convertURIList(p.text)
		if err != nil {
			return nil, "", err
		}
		return out, "uri", nil
	}
	return nil, "", errors.New("unrecognized subscription format (expected sing-box JSON, Clash YAML, or Base64/URI links)")
}

func isClashYAML(data []byte) bool {
	if len(data) == 0 || data[0] == '{' || data[0] == '[' {
		return false
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return false
	}
	return isClashYAMLMap(m)
}

func isClashYAMLMap(m map[string]any) bool {
	if m == nil {
		return false
	}
	for _, k := range []string{"proxies", "proxy-groups", "proxy-providers"} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	if _, hasRules := m["rules"]; hasRules {
		for _, k := range []string{"mixed-port", "port", "socks-port", "mode", "external-controller"} {
			if _, ok := m[k]; ok {
				return true
			}
		}
	}
	return false
}

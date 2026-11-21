package profile

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

func uniqueTag(base string, used map[string]bool) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "node"
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s #%d", base, i)
		if !used[cand] {
			return cand
		}
	}
}

func looksLikeURIList(s string) bool {
	lower := strings.ToLower(s)
	for _, prefix := range []string{"vmess://", "vless://", "trojan://", "ss://", "hysteria2://", "hy2://", "tuic://"} {
		if strings.Contains(lower, prefix) {
			return true
		}
	}
	return false
}

func decodeBase64Flexible(s string) (string, bool) {
	clean := strings.TrimSpace(s)
	clean = strings.ReplaceAll(clean, "\r", "")
	clean = strings.ReplaceAll(clean, "\n", "")
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	}
	for _, enc := range encodings {
		if b, err := enc.DecodeString(clean); err == nil && len(b) > 0 {
			return string(b), true
		}
	}
	return "", false
}

func strVal(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func intVal(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func boolVal(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	default:
		return false
	}
}

func strSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		if ss, ok2 := v.([]string); ok2 {
			return ss
		}
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s := strVal(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func toSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return append([]any(nil), s...)
	}
	return nil
}

func deepMerge(dst, src map[string]any) map[string]any {
	out := make(map[string]any, len(dst))
	for k, v := range dst {
		out[k] = v
	}
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok2 := out[k].(map[string]any); ok2 {
				out[k] = deepMerge(dm, sm)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func appendUniqueStrings(base []string, extras ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(extras))
	out := make([]string, 0, len(base)+len(extras))
	for _, list := range [][]string{base, extras} {
		for _, s := range list {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			k := strings.ToLower(s)
			if _, ok := seen[k]; !ok {
				seen[k] = struct{}{}
				out = append(out, s)
			}
		}
	}
	return out
}

func hasNamedEntry(entries any, key, name string) bool {
	for _, item := range toSlice(entries) {
		entry, _ := item.(map[string]any)
		if entry[key] == name {
			return true
		}
	}
	return false
}

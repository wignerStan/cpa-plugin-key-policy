// Package diagnosticoutput owns bounded sanitized diagnostic views. It never
// enables raw capture or writes access logs, and does not infer disclosure rights.
package diagnosticoutput

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// Fingerprint returns an opt-in integrity identifier. It does not classify secrets.
func Fingerprint(value string) string {
	return fmt.Sprintf("sha256:%x len=%d", sha256.Sum256([]byte(value)), len(value))
}

// CredentialPolicy is immutable through its public methods and safe to share.
// Cookie names are exact and case-sensitive; field names ignore separators/case.
type CredentialPolicy struct {
	fields, cookies map[string]bool
	jwtCookies      bool
}

func EmptyPolicy() CredentialPolicy {
	return CredentialPolicy{fields: map[string]bool{}, cookies: map[string]bool{}}
}
func DefaultPolicy() CredentialPolicy {
	p := EmptyPolicy()
	p.jwtCookies = true
	for _, name := range []string{"authorization", "proxyauthorization", "apikey", "xapikey", "openaiapikey", "accesstoken", "refreshtoken", "idtoken", "authjwt", "jwt", "password", "clientsecret"} {
		p.fields[name] = true
	}
	for _, name := range []string{"session", "sessionid", "session_id", "sid", "connect.sid", "jwt", "access_token", "refresh_token", "id_token", "next-auth.session-token", "__Secure-next-auth.session-token", "__Host-next-auth.session-token", "authjs.session-token", "__Secure-authjs.session-token", "__Host-authjs.session-token"} {
		p.cookies[name] = true
	}
	return p
}
func cloneNames(names map[string]bool) map[string]bool {
	out := make(map[string]bool, len(names)+1)
	for name, v := range names {
		out[name] = v
	}
	return out
}
func (p CredentialPolicy) WithCredentialField(name string) CredentialPolicy {
	p.fields = cloneNames(p.fields)
	p.fields[normalize(name)] = true
	return p
}
func (p CredentialPolicy) WithSessionCookie(name string) CredentialPolicy {
	p.cookies = cloneNames(p.cookies)
	p.cookies[name] = true
	return p
}
func (p CredentialPolicy) WithJWTCookieDetection(enabled bool) CredentialPolicy {
	p.jwtCookies = enabled
	return p
}
func normalize(name string) string {
	return strings.Map(func(r rune) rune {
		if r < 128 && ((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return unicode.ToLower(r)
		}
		return -1
	}, name)
}
func (p CredentialPolicy) CredentialField(name string) bool { return p.fields[normalize(name)] }
func (p CredentialPolicy) sessionCookie(name string) bool {
	if p.cookies[name] {
		return true
	}
	index := strings.LastIndexByte(name, '.')
	if index < 0 {
		return false
	}
	chunk := name[index+1:]
	if chunk == "" {
		return false
	}
	for _, r := range chunk {
		if r < '0' || r > '9' {
			return false
		}
	}
	return p.cookies[name[:index]]
}
func compactJWT(value string) bool {
	value = strings.Trim(strings.TrimSpace(value), "\"")
	parts := strings.Split(value, ".")
	if len(parts) != 3 || !strings.HasPrefix(parts[0], "eyJ") || parts[1] == "" {
		return false
	}
	for _, part := range parts {
		for _, r := range part {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return false
			}
		}
	}
	return true
}
func (p CredentialPolicy) CookieHeader(raw string) string    { return p.cookieHeader(raw, false) }
func (p CredentialPolicy) SetCookieHeader(raw string) string { return p.cookieHeader(raw, true) }
func (p CredentialPolicy) cookieHeader(raw string, set bool) string {
	parts := strings.SplitAfter(raw, ";")
	for index, part := range parts {
		if set && index > 0 {
			continue
		}
		pair := strings.TrimSuffix(part, ";")
		name, value, ok := strings.Cut(pair, "=")
		if ok && (p.sessionCookie(strings.TrimSpace(name)) || p.jwtCookies && compactJWT(value)) {
			parts[index] = name + "=" + Fingerprint(value)
			if strings.HasSuffix(part, ";") {
				parts[index] += ";"
			}
		}
	}
	return strings.Join(parts, "")
}
func (p CredentialPolicy) Header(name, raw string) string {
	field := normalize(name)
	if field == "cookie" {
		return p.CookieHeader(raw)
	}
	if field == "setcookie" {
		return p.SetCookieHeader(raw)
	}
	if p.CredentialField(name) {
		return fingerprintString(raw, field == "authorization" || field == "proxyauthorization")
	}
	return raw
}
func fingerprintString(value string, authorization bool) string {
	if authorization {
		scheme, credential, ok := strings.Cut(value, " ")
		for _, known := range []string{"bearer", "basic", "digest", "negotiate"} {
			if ok && strings.EqualFold(scheme, known) {
				return scheme + " " + Fingerprint(credential)
			}
		}
	}
	return Fingerprint(value)
}
func fingerprintTree(value any, authorization bool) any {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return fingerprintString(v, authorization)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = fingerprintTree(item, authorization)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			out[k] = fingerprintTree(item, authorization)
		}
		return out
	default:
		bytes, err := json.Marshal(v)
		if err != nil {
			return Fingerprint("unsupported classified value")
		}
		return Fingerprint(string(bytes))
	}
}

// Redact builds a view of JSON-native values without modifying their owner.
// Arbitrary struct types must be projected/decoded into JSON-native values first.
func (p CredentialPolicy) Redact(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for name, item := range v {
			field := normalize(name)
			switch {
			case p.fields[field]:
				out[name] = fingerprintTree(item, field == "authorization" || field == "proxyauthorization")
			case field == "cookie" || field == "cookies" || field == "setcookie" || field == "setcookies":
				out[name] = p.cookieContainer(item, strings.HasPrefix(field, "set"))
			default:
				out[name] = p.Redact(item)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = p.Redact(item)
		}
		return out
	default:
		return value
	}
}
func (p CredentialPolicy) cookieValue(name string, value any) any {
	if p.sessionCookie(name) {
		return fingerprintTree(value, false)
	}
	if p.jwtCookies {
		return fingerprintCookieJWTs(value)
	}
	return copyCookieValue(value, false)
}
func fingerprintCookieJWTs(value any) any { return copyCookieValue(value, true) }

// Copy ordinary cookie values too, so diagnostic views never share mutable trees.
func copyCookieValue(value any, detectJWT bool) any {
	switch v := value.(type) {
	case string:
		if detectJWT && compactJWT(v) {
			return Fingerprint(v)
		}
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = copyCookieValue(item, detectJWT)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for name, item := range v {
			out[name] = copyCookieValue(item, detectJWT)
		}
		return out
	}
	return value
}
func cookieName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
			return false
		}
	}
	return true
}
func (p CredentialPolicy) cookieContainer(value any, set bool) any {
	switch v := value.(type) {
	case string:
		return p.cookieHeader(v, set)
	case []any:
		if len(v) == 2 {
			if name, ok := v[0].(string); ok && cookieName(name) {
				return []any{name, p.cookieValue(name, v[1])}
			}
		}
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = p.cookieContainer(item, set)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		if name, ok := v["name"].(string); ok {
			if _, exists := v["value"]; exists {
				for key, item := range v {
					if key == "value" {
						out[key] = p.cookieValue(name, item)
					} else {
						out[key] = copyCookieValue(item, false)
					}
				}
				return out
			}
		}
		for name, item := range v {
			out[name] = p.cookieValue(name, item)
		}
		return out
	default:
		return value
	}
}

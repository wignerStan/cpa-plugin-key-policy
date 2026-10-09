package diagnosticoutput

import "strings"

// Projection selects diagnostic fields without granting visibility to whole bodies.
// Its copied rules are immutable and safe to share. Names ignore case and space,
// but do not use substring matching. A container permits all its descendants.
// CredentialPolicy still classifies credentials within permitted containers.
type Projection struct {
	fields, containers map[string]bool
}

// NewProjection copies the caller's visible field and metadata container names.
// An empty selection includes only explicitly classified credential fields.
func NewProjection(fields, containers []string) Projection {
	p := Projection{fields: make(map[string]bool, len(fields)), containers: make(map[string]bool, len(containers))}
	for _, name := range fields {
		p.fields[projectionName(name)] = true
	}
	for _, name := range containers {
		p.containers[projectionName(name)] = true
	}
	return p
}

func projectionName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// Project copies selected JSON-native values and classifies credentials in one
// traversal. Visible nonsecret values remain readable, including JWT-shaped turn
// state. It performs no I/O. Decode arbitrary typed values before calling it.
func (p Projection) Project(value any, policy CredentialPolicy) any {
	return p.project(value, policy, false)
}

func (p Projection) project(value any, policy CredentialPolicy, inContainer bool) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any)
		for name, item := range v {
			field := normalize(name)
			selected := projectionName(name)
			switch {
			case policy.CredentialField(name):
				out[name] = fingerprintTree(item, field == "authorization" || field == "proxyauthorization")
			case inContainer || p.fields[selected] || p.containers[selected]:
				if field == "cookie" || field == "cookies" || field == "setcookie" || field == "setcookies" {
					out[name] = policy.cookieContainer(item, strings.HasPrefix(field, "set"))
				} else {
					out[name] = p.project(item, policy, inContainer || p.containers[selected])
				}
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = p.project(item, policy, inContainer)
		}
		return out
	default:
		return value
	}
}

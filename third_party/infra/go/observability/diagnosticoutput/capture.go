package diagnosticoutput

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

// Result carries metadata plus a complete sanitized detail, never a raw prefix.
type Result struct {
	Outcome       string
	ObservedBytes uint64
	Detail        []byte
}

// Capture is request-local, deliberately unsynchronized. Chunk observation only
// counts/copies bounded private memory. Assembly selects whether/how to emit its
// single final result; this type performs no logging, queue sends or I/O.
type Capture struct {
	limit    int
	bytes    []byte
	observed uint64
	format   string
	settled  bool
	policy   *CredentialPolicy
}

func NewCapture(limit int) *Capture {
	if limit < 0 {
		limit = 0
	}
	return &Capture{limit: limit}
}
func (c *Capture) SetCredentialPolicy(policy CredentialPolicy) { c.policy = &policy }
func (c *Capture) SetContentType(contentType string) {
	media, _, _ := strings.Cut(contentType, ";")
	media = strings.ToLower(strings.TrimSpace(media))
	switch {
	case media == "application/json" || strings.HasPrefix(media, "application/") && strings.HasSuffix(media, "+json"):
		c.format = "json"
	case media == "text/event-stream":
		c.format = "sse"
	default:
		c.format = ""
	}
}
func (c *Capture) Observe(chunk []byte) {
	if c == nil || c.limit == 0 || c.settled {
		return
	}
	if uint64(len(chunk)) > ^uint64(0)-c.observed {
		c.observed = ^uint64(0)
	} else {
		c.observed += uint64(len(chunk))
	}
	if c.observed > uint64(c.limit) {
		c.bytes = nil
		return
	}
	if c.format != "" {
		c.bytes = append(c.bytes, chunk...)
	}
}
func (c *Capture) Finish(complete bool) Result {
	if c == nil || c.limit == 0 {
		return Result{Outcome: "disabled"}
	}
	result := Result{ObservedBytes: c.observed}
	if c.settled {
		result.Outcome = "already_finished"
		return result
	}
	c.settled = true
	input := c.bytes
	c.bytes = nil
	switch {
	case !complete:
		result.Outcome = "incomplete"
		return result
	case c.observed > uint64(c.limit):
		result.Outcome = "size_limit"
		return result
	case c.format == "":
		result.Outcome = "unsupported_content_type"
		return result
	}
	var value any
	var err error
	if c.format == "json" {
		value, err = decodeJSON(input)
	} else {
		value, err = decodeSSE(input)
	}
	if err != nil {
		result.Outcome = err.Error()
		return result
	}
	if c.policy == nil {
		policy := DefaultPolicy()
		c.policy = &policy
	}
	result.Detail, err = encodeBounded(c.policy.Redact(value), c.limit)
	if err != nil {
		result.Outcome = "encoded_size_limit"
		return result
	}
	result.Outcome = "captured"
	return result
}
func decodeJSON(input []byte) (any, error) {
	if !utf8.Valid(input) {
		return nil, errors.New("not_json")
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, errors.New("not_json")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("not_json")
	}
	return value, nil
}
func (p CredentialPolicy) SanitizeJSON(input []byte, limit int) ([]byte, error) {
	if limit <= 0 || len(input) > limit {
		return nil, errors.New("size_limit")
	}
	value, err := decodeJSON(input)
	if err != nil {
		return nil, err
	}
	return encodeBounded(p.Redact(value), limit)
}
func decodeSSE(input []byte) (any, error) {
	if !utf8.Valid(input) {
		return nil, errors.New("not_sse_json")
	}
	text := strings.TrimPrefix(string(input), "\ufeff")
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	events := make([]any, 0)
	var data strings.Builder
	hasData, pending, done := false, false, false
	lines := strings.Split(text, "\n")
	if strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		if line == "" {
			if hasData {
				if done {
					return nil, errors.New("not_sse_json")
				}
				payload := data.String()
				if payload == "[DONE]" {
					done = true
					events = append(events, payload)
				} else {
					value, err := decodeJSON([]byte(payload))
					if err != nil {
						return nil, errors.New("not_sse_json")
					}
					events = append(events, value)
				}
				data.Reset()
				hasData = false
			}
			pending = false
		} else if !strings.HasPrefix(line, ":") {
			pending = true
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "data":
				if hasData {
					data.WriteByte('\n')
				}
				data.WriteString(value)
				hasData = true
			case "event", "id", "retry":
			default:
				return nil, errors.New("not_sse_json")
			}
		}
	}
	if hasData || pending {
		return nil, errors.New("incomplete_sse")
	}
	if len(events) == 0 {
		return nil, errors.New("not_sse_json")
	}
	return events, nil
}

// Stream each JSON-native node into a bounded private buffer. json.Encoder would
// assemble an unbounded encoded record before invoking a limited writer.
func encodeBounded(value any, limit int) ([]byte, error) {
	var out bytes.Buffer
	appendBytes := func(data []byte) error {
		if len(data) > limit-out.Len() {
			return errors.New("encoded_size_limit")
		}
		_, err := out.Write(data)
		return err
	}
	var encode func(any) error
	encode = func(value any) error {
		switch v := value.(type) {
		case map[string]any:
			if err := appendBytes([]byte("{")); err != nil {
				return err
			}
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for i, key := range keys {
				if i > 0 {
					if err := appendBytes([]byte(",")); err != nil {
						return err
					}
				}
				name, err := json.Marshal(key)
				if err != nil {
					return err
				}
				if err = appendBytes(name); err != nil {
					return err
				}
				if err = appendBytes([]byte(":")); err != nil {
					return err
				}
				if err = encode(v[key]); err != nil {
					return err
				}
			}
			return appendBytes([]byte("}"))
		case []any:
			if err := appendBytes([]byte("[")); err != nil {
				return err
			}
			for i, item := range v {
				if i > 0 {
					if err := appendBytes([]byte(",")); err != nil {
						return err
					}
				}
				if err := encode(item); err != nil {
					return err
				}
			}
			return appendBytes([]byte("]"))
		default:
			encoded, err := json.Marshal(value)
			if err != nil {
				return err
			}
			return appendBytes(encoded)
		}
	}
	if err := encode(value); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

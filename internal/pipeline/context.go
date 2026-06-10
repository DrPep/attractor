package pipeline

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// PipelineContext is a thread-safe key-value store shared across stages.
// It supports dotted keys (e.g. "step1.result") that nest into sub-maps.
type PipelineContext struct {
	mu   sync.Mutex
	data map[string]any
}

// NewPipelineContext returns a context optionally seeded with initial data.
func NewPipelineContext(initial map[string]any) *PipelineContext {
	data := map[string]any{}
	for k, v := range initial {
		data[k] = v
	}
	return &PipelineContext{data: data}
}

// Get returns the value at a dotted key, or def if absent.
func (c *PipelineContext) Get(key string, def any) any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getNested(key, def)
}

// GetString returns the value at a dotted key as a string ("" if absent/nil).
func (c *PipelineContext) GetString(key string) string {
	v := c.Get(key, nil)
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// Set assigns a value at a dotted key.
func (c *PipelineContext) Set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setNested(key, value)
}

// Update merges a flat dotted map into the context.
func (c *PipelineContext) Update(m map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range m {
		c.setNested(k, v)
	}
}

// Snapshot returns a deep copy of all data.
func (c *PipelineContext) Snapshot() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return deepCopyMap(c.data)
}

// Restore replaces all data with a deep copy of the snapshot.
func (c *PipelineContext) Restore(snapshot map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = deepCopyMap(snapshot)
}

// Keys returns all flattened dotted leaf keys, sorted.
func (c *PipelineContext) Keys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	collectKeys(c.data, "", &out)
	sort.Strings(out)
	return out
}

func (c *PipelineContext) getNested(key string, def any) any {
	parts := strings.Split(key, ".")
	var cur any = c.data
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return def
		}
		v, present := m[p]
		if !present {
			return def
		}
		cur = v
	}
	return cur
}

func (c *PipelineContext) setNested(key string, value any) {
	parts := strings.Split(key, ".")
	cur := c.data
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = value
}

func collectKeys(data map[string]any, prefix string, out *[]string) {
	for k, v := range data {
		full := k
		if prefix != "" {
			full = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			collectKeys(sub, full, out)
		} else {
			*out = append(*out, full)
		}
	}
}

func deepCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyValue(v)
	}
	return out
}

func deepCopyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return deepCopyMap(t)
	case []any:
		s := make([]any, len(t))
		for i, e := range t {
			s[i] = deepCopyValue(e)
		}
		return s
	default:
		return v
	}
}

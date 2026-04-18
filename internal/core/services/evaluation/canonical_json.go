package evaluation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// orderedMap implements json.Marshaler to produce JSON with sorted keys.
// This ensures deterministic output for map[string]any regardless of Go's random map iteration order.
type orderedMap struct {
	pairs [][2]any
}

// MarshalJSON produces a JSON object with keys in the order stored in pairs.
func (o orderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, pair := range o.pairs {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(pair[0])
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(pair[1])
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// canonicalizeValue recursively transforms a value to use orderedMap for all maps,
// ensuring consistent key ordering during JSON serialization.
func canonicalizeValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		if len(val) == 0 {
			return val
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		pairs := make([][2]any, len(keys))
		for i, k := range keys {
			pairs[i] = [2]any{k, canonicalizeValue(val[k])}
		}
		return orderedMap{pairs: pairs}
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = canonicalizeValue(item)
		}
		return result
	default:
		return v
	}
}

// CanonicalJSONMarshal produces deterministic JSON with sorted map keys at all nesting levels.
// This is essential for content hashing where identical data must produce identical hashes.
func CanonicalJSONMarshal(v any) ([]byte, error) {
	return json.Marshal(canonicalizeValue(v))
}

// ComputeContentHash computes a deterministic SHA256 hash of input and expected fields.
// Uses canonical JSON with sorted keys to ensure identical data produces identical hashes,
// regardless of the original map iteration order.
func ComputeContentHash(input, expected map[string]any) string {
	// Build data structure with keys in alphabetical order for consistency
	data := map[string]any{
		"expected": expected,
		"input":    input,
	}
	jsonBytes, err := CanonicalJSONMarshal(data)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(jsonBytes)
	return hex.EncodeToString(hash[:])
}

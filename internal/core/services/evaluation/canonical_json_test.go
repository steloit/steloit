package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeContentHash_Deterministic(t *testing.T) {
	input := map[string]any{
		"z_key": "value1",
		"a_key": "value2",
		"m_key": map[string]any{
			"nested_z": 1,
			"nested_a": 2,
		},
	}
	expected := map[string]any{
		"output": "result",
	}

	hash1 := ComputeContentHash(input, expected)
	hash2 := ComputeContentHash(input, expected)
	hash3 := ComputeContentHash(input, expected)

	assert.Equal(t, hash1, hash2, "Hash should be deterministic across calls")
	assert.Equal(t, hash2, hash3, "Hash should be deterministic across calls")
	assert.NotEmpty(t, hash1, "Hash should not be empty")
}

func TestComputeContentHash_SameDataDifferentMapOrder(t *testing.T) {
	input1 := map[string]any{
		"a": 1,
		"b": 2,
		"c": 3,
	}
	expected1 := map[string]any{
		"x": "foo",
		"y": "bar",
	}

	input2 := make(map[string]any)
	input2["c"] = 3
	input2["a"] = 1
	input2["b"] = 2

	expected2 := make(map[string]any)
	expected2["y"] = "bar"
	expected2["x"] = "foo"

	hash1 := ComputeContentHash(input1, expected1)
	hash2 := ComputeContentHash(input2, expected2)

	assert.Equal(t, hash1, hash2, "Same logical data should produce same hash")
}

func TestComputeContentHash_DifferentDataDifferentHash(t *testing.T) {
	input1 := map[string]any{"key": "value1"}
	expected1 := map[string]any{"output": "result1"}

	input2 := map[string]any{"key": "value2"}
	expected2 := map[string]any{"output": "result2"}

	hash1 := ComputeContentHash(input1, expected1)
	hash2 := ComputeContentHash(input2, expected2)

	assert.NotEqual(t, hash1, hash2, "Different data should produce different hash")
}

func TestComputeContentHash_EmptyMaps(t *testing.T) {
	input := map[string]any{}
	expected := map[string]any{}

	hash := ComputeContentHash(input, expected)
	assert.NotEmpty(t, hash, "Hash should not be empty for empty maps")

	hash2 := ComputeContentHash(input, expected)
	assert.Equal(t, hash, hash2)
}

func TestComputeContentHash_NilMaps(t *testing.T) {
	var input map[string]any
	var expected map[string]any

	hash := ComputeContentHash(input, expected)
	assert.NotEmpty(t, hash, "Hash should not be empty for nil maps")
}

func TestComputeContentHash_NestedStructures(t *testing.T) {
	input := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"z_key": "deep",
				"a_key": "value",
			},
			"array": []any{
				map[string]any{
					"z": 1,
					"a": 2,
				},
			},
		},
	}
	expected := map[string]any{
		"output": "nested_result",
	}

	hash1 := ComputeContentHash(input, expected)
	hash2 := ComputeContentHash(input, expected)

	assert.Equal(t, hash1, hash2)
	assert.NotEmpty(t, hash1)
}

func TestCanonicalJSONMarshal_SortedKeys(t *testing.T) {
	data := map[string]any{
		"z": 1,
		"a": 2,
		"m": 3,
	}

	bytes1, err := CanonicalJSONMarshal(data)
	assert.NoError(t, err)

	expected := `{"a":2,"m":3,"z":1}`
	assert.Equal(t, expected, string(bytes1))
}

func TestCanonicalJSONMarshal_NestedMaps(t *testing.T) {
	data := map[string]any{
		"outer_z": map[string]any{
			"inner_z": 1,
			"inner_a": 2,
		},
		"outer_a": "value",
	}

	bytes, err := CanonicalJSONMarshal(data)
	assert.NoError(t, err)

	expected := `{"outer_a":"value","outer_z":{"inner_a":2,"inner_z":1}}`
	assert.Equal(t, expected, string(bytes))
}

func TestCanonicalJSONMarshal_Arrays(t *testing.T) {
	data := map[string]any{
		"array": []any{
			map[string]any{
				"z": 1,
				"a": 2,
			},
			map[string]any{
				"y": 3,
				"b": 4,
			},
		},
	}

	bytes, err := CanonicalJSONMarshal(data)
	assert.NoError(t, err)

	expected := `{"array":[{"a":2,"z":1},{"b":4,"y":3}]}`
	assert.Equal(t, expected, string(bytes))
}

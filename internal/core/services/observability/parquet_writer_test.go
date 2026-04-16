package observability

import (
	"testing"
	"time"

	"github.com/parquet-go/parquet-go/compress/zstd"
	"github.com/stretchr/testify/assert"

	"brokle/internal/core/domain/observability"
)

func TestParquetWriter_GetZstdLevel(t *testing.T) {
	tests := []struct {
		name             string
		compressionLevel int
		expectedLevel    zstd.Level
	}{
		// SpeedFastest for level 1
		{
			name:             "level 1 returns SpeedFastest",
			compressionLevel: 1,
			expectedLevel:    zstd.SpeedFastest,
		},
		// SpeedDefault for levels 2-3
		{
			name:             "level 2 returns SpeedDefault",
			compressionLevel: 2,
			expectedLevel:    zstd.SpeedDefault,
		},
		{
			name:             "level 3 returns SpeedDefault",
			compressionLevel: 3,
			expectedLevel:    zstd.SpeedDefault,
		},
		// SpeedBetterCompression for levels 4-9
		{
			name:             "level 4 returns SpeedBetterCompression",
			compressionLevel: 4,
			expectedLevel:    zstd.SpeedBetterCompression,
		},
		{
			name:             "level 9 returns SpeedBetterCompression",
			compressionLevel: 9,
			expectedLevel:    zstd.SpeedBetterCompression,
		},
		// SpeedBestCompression for levels 10+
		{
			name:             "level 10 returns SpeedBestCompression",
			compressionLevel: 10,
			expectedLevel:    zstd.SpeedBestCompression,
		},
		{
			name:             "level 22 returns SpeedBestCompression",
			compressionLevel: 22,
			expectedLevel:    zstd.SpeedBestCompression,
		},
		// Edge cases (clamped by NewParquetWriter, but test the mapping)
		{
			name:             "level 0 clamped to 1 returns SpeedFastest",
			compressionLevel: 1, // NewParquetWriter clamps 0 to 1
			expectedLevel:    zstd.SpeedFastest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := NewParquetWriter(tt.compressionLevel)
			level := writer.getZstdLevel()
			assert.Equal(t, tt.expectedLevel, level)
		})
	}
}

func TestNewParquetWriter_ClampsCompressionLevel(t *testing.T) {
	tests := []struct {
		name            string
		inputLevel      int
		expectedClamped int
	}{
		{
			name:            "level 0 clamped to 1",
			inputLevel:      0,
			expectedClamped: 1,
		},
		{
			name:            "negative level clamped to 1",
			inputLevel:      -5,
			expectedClamped: 1,
		},
		{
			name:            "level 23 clamped to 22",
			inputLevel:      23,
			expectedClamped: 22,
		},
		{
			name:            "level 100 clamped to 22",
			inputLevel:      100,
			expectedClamped: 22,
		},
		{
			name:            "valid level 3 unchanged",
			inputLevel:      3,
			expectedClamped: 3,
		},
		{
			name:            "valid level 15 unchanged",
			inputLevel:      15,
			expectedClamped: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := NewParquetWriter(tt.inputLevel)
			assert.Equal(t, tt.expectedClamped, writer.compressionLevel)
		})
	}
}

func TestParquetWriter_WriteRecords(t *testing.T) {
	t.Run("empty records returns error", func(t *testing.T) {
		writer := NewParquetWriter(3)
		data, err := writer.WriteRecords(nil)
		assert.Error(t, err)
		assert.Nil(t, data)
		assert.Contains(t, err.Error(), "no records to write")
	})

	t.Run("empty slice returns error", func(t *testing.T) {
		writer := NewParquetWriter(3)
		data, err := writer.WriteRecords([]observability.RawTelemetryRecord{})
		assert.Error(t, err)
		assert.Nil(t, data)
	})

	t.Run("single record writes successfully", func(t *testing.T) {
		writer := NewParquetWriter(3)
		records := []observability.RawTelemetryRecord{
			{
				RecordID:    "rec-001",
				ProjectID:   "proj-001",
				SignalType:  observability.SignalTypeTraces,
				Timestamp:   time.Now(),
				TraceID:     "trace-001",
				SpanID:      "span-001",
				SpanJSONRaw: `{"name": "test"}`,
				ArchivedAt:  time.Now(),
			},
		}

		data, err := writer.WriteRecords(records)
		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Greater(t, len(data), 0)
	})

	t.Run("multiple records write successfully", func(t *testing.T) {
		writer := NewParquetWriter(3)
		now := time.Now()
		records := []observability.RawTelemetryRecord{
			{
				RecordID:    "rec-001",
				ProjectID:   "proj-001",
				SignalType:  observability.SignalTypeTraces,
				Timestamp:   now,
				TraceID:     "trace-001",
				SpanID:      "span-001",
				SpanJSONRaw: `{"name": "span1"}`,
				ArchivedAt:  now,
			},
			{
				RecordID:    "rec-002",
				ProjectID:   "proj-001",
				SignalType:  observability.SignalTypeTraces,
				Timestamp:   now.Add(time.Second),
				TraceID:     "trace-001",
				SpanID:      "span-002",
				SpanJSONRaw: `{"name": "span2"}`,
				ArchivedAt:  now,
			},
		}

		data, err := writer.WriteRecords(records)
		assert.NoError(t, err)
		assert.NotNil(t, data)
		assert.Greater(t, len(data), 0)
	})
}

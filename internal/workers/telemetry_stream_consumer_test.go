package workers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"brokle/internal/core/domain/observability"
)

func TestTelemetryStreamConsumer_GroupRecordsBySignalAndDay(t *testing.T) {
	// Create minimal consumer for testing (method doesn't use any dependencies)
	consumer := &TelemetryStreamConsumer{}

	// Fixed dates for testing
	day1 := time.Date(2024, 12, 15, 10, 30, 0, 0, time.UTC)
	day2 := time.Date(2024, 12, 16, 14, 45, 0, 0, time.UTC)
	day1Late := time.Date(2024, 12, 15, 23, 59, 59, 0, time.UTC)
	day2Early := time.Date(2024, 12, 16, 0, 0, 1, 0, time.UTC)

	tests := []struct {
		name            string
		records         []observability.RawTelemetryRecord
		expectedSignals int
		expectedDays    map[string]int            // signal -> number of day groups
		expectedCounts  map[string]map[string]int // signal -> day -> count
	}{
		{
			name:            "empty records returns empty map",
			records:         []observability.RawTelemetryRecord{},
			expectedSignals: 0,
			expectedDays:    map[string]int{},
			expectedCounts:  map[string]map[string]int{},
		},
		{
			name: "single signal single day",
			records: []observability.RawTelemetryRecord{
				{RecordID: "1", SignalType: observability.SignalTypeTraces, Timestamp: day1},
				{RecordID: "2", SignalType: observability.SignalTypeTraces, Timestamp: day1},
			},
			expectedSignals: 1,
			expectedDays:    map[string]int{observability.SignalTypeTraces: 1},
			expectedCounts: map[string]map[string]int{
				observability.SignalTypeTraces: {"2024-12-15": 2},
			},
		},
		{
			name: "single signal multiple days",
			records: []observability.RawTelemetryRecord{
				{RecordID: "1", SignalType: observability.SignalTypeTraces, Timestamp: day1},
				{RecordID: "2", SignalType: observability.SignalTypeTraces, Timestamp: day2},
				{RecordID: "3", SignalType: observability.SignalTypeTraces, Timestamp: day1},
			},
			expectedSignals: 1,
			expectedDays:    map[string]int{observability.SignalTypeTraces: 2},
			expectedCounts: map[string]map[string]int{
				observability.SignalTypeTraces: {"2024-12-15": 2, "2024-12-16": 1},
			},
		},
		{
			name: "multiple signals single day",
			records: []observability.RawTelemetryRecord{
				{RecordID: "1", SignalType: observability.SignalTypeTraces, Timestamp: day1},
				{RecordID: "2", SignalType: observability.SignalTypeLogs, Timestamp: day1},
				{RecordID: "3", SignalType: observability.SignalTypeMetrics, Timestamp: day1},
			},
			expectedSignals: 3,
			expectedDays: map[string]int{
				observability.SignalTypeTraces:  1,
				observability.SignalTypeLogs:    1,
				observability.SignalTypeMetrics: 1,
			},
			expectedCounts: map[string]map[string]int{
				observability.SignalTypeTraces:  {"2024-12-15": 1},
				observability.SignalTypeLogs:    {"2024-12-15": 1},
				observability.SignalTypeMetrics: {"2024-12-15": 1},
			},
		},
		{
			name: "multiple signals multiple days",
			records: []observability.RawTelemetryRecord{
				{RecordID: "1", SignalType: observability.SignalTypeTraces, Timestamp: day1},
				{RecordID: "2", SignalType: observability.SignalTypeTraces, Timestamp: day2},
				{RecordID: "3", SignalType: observability.SignalTypeLogs, Timestamp: day1},
				{RecordID: "4", SignalType: observability.SignalTypeLogs, Timestamp: day2},
				{RecordID: "5", SignalType: observability.SignalTypeMetrics, Timestamp: day1},
			},
			expectedSignals: 3,
			expectedDays: map[string]int{
				observability.SignalTypeTraces:  2,
				observability.SignalTypeLogs:    2,
				observability.SignalTypeMetrics: 1,
			},
			expectedCounts: map[string]map[string]int{
				observability.SignalTypeTraces:  {"2024-12-15": 1, "2024-12-16": 1},
				observability.SignalTypeLogs:    {"2024-12-15": 1, "2024-12-16": 1},
				observability.SignalTypeMetrics: {"2024-12-15": 1},
			},
		},
		{
			name: "midnight boundary - records on different sides of midnight",
			records: []observability.RawTelemetryRecord{
				{RecordID: "1", SignalType: observability.SignalTypeTraces, Timestamp: day1Late},  // 23:59:59 on Dec 15
				{RecordID: "2", SignalType: observability.SignalTypeTraces, Timestamp: day2Early}, // 00:00:01 on Dec 16
			},
			expectedSignals: 1,
			expectedDays:    map[string]int{observability.SignalTypeTraces: 2},
			expectedCounts: map[string]map[string]int{
				observability.SignalTypeTraces: {"2024-12-15": 1, "2024-12-16": 1},
			},
		},
		{
			name: "genai signal type",
			records: []observability.RawTelemetryRecord{
				{RecordID: "1", SignalType: observability.SignalTypeGenAI, Timestamp: day1},
				{RecordID: "2", SignalType: observability.SignalTypeGenAI, Timestamp: day1},
			},
			expectedSignals: 1,
			expectedDays:    map[string]int{observability.SignalTypeGenAI: 1},
			expectedCounts: map[string]map[string]int{
				observability.SignalTypeGenAI: {"2024-12-15": 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := consumer.groupRecordsBySignalAndDay(tt.records)

			// Check number of signal types
			assert.Equal(t, tt.expectedSignals, len(result), "unexpected number of signal types")

			// Check number of days per signal
			for signal, expectedDayCount := range tt.expectedDays {
				dayGroups, exists := result[signal]
				assert.True(t, exists, "missing signal type: %s", signal)
				assert.Equal(t, expectedDayCount, len(dayGroups), "unexpected number of days for signal %s", signal)
			}

			// Check record counts per signal and day
			for signal, dayCounts := range tt.expectedCounts {
				dayGroups, exists := result[signal]
				assert.True(t, exists, "missing signal type: %s", signal)
				for day, expectedCount := range dayCounts {
					records, dayExists := dayGroups[day]
					assert.True(t, dayExists, "missing day %s for signal %s", day, signal)
					assert.Equal(t, expectedCount, len(records), "unexpected count for signal %s, day %s", signal, day)
				}
			}
		})
	}
}

func TestTelemetryStreamConsumer_GroupRecordsBySignalAndDay_PreservesRecordData(t *testing.T) {
	consumer := &TelemetryStreamConsumer{}

	day1 := time.Date(2024, 12, 15, 10, 30, 0, 0, time.UTC)

	records := []observability.RawTelemetryRecord{
		{
			RecordID:    "rec-001",
			ProjectID:   "proj-001",
			SignalType:  observability.SignalTypeTraces,
			Timestamp:   day1,
			TraceID:     "trace-001",
			SpanID:      "span-001",
			SpanJSONRaw: `{"name": "test-span"}`,
			ArchivedAt:  day1,
		},
	}

	result := consumer.groupRecordsBySignalAndDay(records)

	// Verify record data is preserved
	assert.Len(t, result, 1)
	tracesGroups := result[observability.SignalTypeTraces]
	assert.Len(t, tracesGroups, 1)

	dayRecords := tracesGroups["2024-12-15"]
	assert.Len(t, dayRecords, 1)

	record := dayRecords[0]
	assert.Equal(t, "rec-001", record.RecordID)
	assert.Equal(t, "proj-001", record.ProjectID)
	assert.Equal(t, "trace-001", record.TraceID)
	assert.Equal(t, "span-001", record.SpanID)
	assert.Equal(t, `{"name": "test-span"}`, record.SpanJSONRaw)
}

package dashboard

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"time"

	dashboardDomain "brokle/internal/core/domain/dashboard"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/shopspring/decimal"
)

type widgetQueryRepository struct {
	db clickhouse.Conn
}

// NewWidgetQueryRepository creates a new widget query repository
func NewWidgetQueryRepository(db clickhouse.Conn) dashboardDomain.WidgetQueryRepository {
	return &widgetQueryRepository{db: db}
}

// ExecuteQuery executes a raw query and returns results as maps
func (r *widgetQueryRepository) ExecuteQuery(
	ctx context.Context,
	query string,
	args []any,
) ([]map[string]any, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	// Get column names and types
	columnTypes := rows.ColumnTypes()
	columnNames := make([]string, len(columnTypes))
	for i, ct := range columnTypes {
		columnNames[i] = ct.Name()
	}

	results := make([]map[string]any, 0)

	for rows.Next() {
		// Create typed pointers based on column types to satisfy ClickHouse driver requirements
		valuePtrs := make([]any, len(columnTypes))
		for i, ct := range columnTypes {
			valuePtrs[i] = createScanTarget(ct.ScanType())
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		// Convert to map with dereferenced and type-converted values
		row := make(map[string]any)
		for i, name := range columnNames {
			row[name] = convertScannedValue(valuePtrs[i])
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return results, nil
}

// ExecuteTraceListQuery executes a trace list query
func (r *widgetQueryRepository) ExecuteTraceListQuery(
	ctx context.Context,
	query string,
	args []any,
) ([]*dashboardDomain.TraceListItem, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("execute trace list query: %w", err)
	}
	defer rows.Close()

	traces := make([]*dashboardDomain.TraceListItem, 0)

	for rows.Next() {
		var trace dashboardDomain.TraceListItem
		var durationNano *uint64
		var totalCost *decimal.Decimal
		var statusCode *uint8

		if err := rows.Scan(
			&trace.TraceID,
			&trace.Name,
			&trace.StartTime,
			&durationNano,
			&statusCode,
			&totalCost,
			&trace.ModelName,
			&trace.ProviderName,
			&trace.ServiceName,
		); err != nil {
			return nil, fmt.Errorf("scan trace row: %w", err)
		}

		if durationNano != nil {
			trace.DurationNano = int64(*durationNano)
		}
		if statusCode != nil {
			trace.StatusCode = int(*statusCode)
		}
		if totalCost != nil {
			cost, _ := totalCost.Float64()
			trace.TotalCost = &cost
		}

		traces = append(traces, &trace)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trace rows: %w", err)
	}

	return traces, nil
}

// ExecuteHistogramQuery executes a histogram query
func (r *widgetQueryRepository) ExecuteHistogramQuery(
	ctx context.Context,
	query string,
	args []any,
) (*dashboardDomain.HistogramData, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("execute histogram query: %w", err)
	}
	defer rows.Close()

	result := &dashboardDomain.HistogramData{
		Buckets: make([]dashboardDomain.HistogramBucket, 0),
	}

	// ClickHouse histogram() returns an array of tuples (lower, upper, count)
	for rows.Next() {
		var buckets [][]float64

		if err := rows.Scan(&buckets); err != nil {
			return nil, fmt.Errorf("scan histogram row: %w", err)
		}

		for _, bucket := range buckets {
			if len(bucket) >= 3 {
				result.Buckets = append(result.Buckets, dashboardDomain.HistogramBucket{
					LowerBound: bucket[0],
					UpperBound: bucket[1],
					Count:      int64(bucket[2]),
				})
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate histogram rows: %w", err)
	}

	return result, nil
}

// convertValue converts ClickHouse values to JSON-friendly types.
// It ensures all returned values can be safely marshaled to JSON.
func convertValue(v any) any {
	switch val := v.(type) {
	// Time types
	case time.Time:
		return val.Format(time.RFC3339)
	case *time.Time:
		if val != nil {
			return val.Format(time.RFC3339)
		}
		return nil

	// Decimal types (shopspring/decimal)
	case decimal.Decimal:
		f, _ := val.Float64()
		return f
	case *decimal.Decimal:
		if val != nil {
			f, _ := val.Float64()
			return f
		}
		return nil

	// Integer types
	case int:
		return val
	case *int:
		if val != nil {
			return *val
		}
		return nil
	case int8:
		return int(val)
	case *int8:
		if val != nil {
			return int(*val)
		}
		return nil
	case int16:
		return int(val)
	case *int16:
		if val != nil {
			return int(*val)
		}
		return nil
	case int32:
		return val
	case *int32:
		if val != nil {
			return *val
		}
		return nil
	case int64:
		return val
	case *int64:
		if val != nil {
			return *val
		}
		return nil
	case uint:
		return val
	case *uint:
		if val != nil {
			return *val
		}
		return nil
	case uint8:
		return int(val)
	case *uint8:
		if val != nil {
			return int(*val)
		}
		return nil
	case uint16:
		return int(val)
	case *uint16:
		if val != nil {
			return int(*val)
		}
		return nil
	case uint32:
		return val
	case *uint32:
		if val != nil {
			return *val
		}
		return nil
	case uint64:
		return val
	case *uint64:
		if val != nil {
			return *val
		}
		return nil

	// Float types - handle NaN/Infinity which JSON cannot represent
	case float32:
		f := float64(val)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return f
	case *float32:
		if val != nil {
			f := float64(*val)
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return nil
			}
			return f
		}
		return nil
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil
		}
		return val
	case *float64:
		if val != nil {
			if math.IsNaN(*val) || math.IsInf(*val, 0) {
				return nil
			}
			return *val
		}
		return nil

	// String and byte types
	case string:
		return val
	case *string:
		if val != nil {
			return *val
		}
		return nil
	case []byte:
		return string(val)

	// Boolean
	case bool:
		return val
	case *bool:
		if val != nil {
			return *val
		}
		return nil

	// Nil
	case nil:
		return nil

	// Fallback: return value as-is for types not explicitly handled
	default:
		return val
	}
}

// createScanTarget creates a properly typed pointer based on reflect.Type.
// This is necessary because ClickHouse's Go driver requires explicit typed pointers
// for scanning aggregate function results (UInt64, Float64, etc.).
func createScanTarget(t reflect.Type) any {
	if t == nil {
		return new(any)
	}
	return reflect.New(t).Interface()
}

// convertScannedValue dereferences a scanned pointer and converts to JSON-friendly type.
// It uses reflection to handle dynamically typed scan targets created by createScanTarget.
func convertScannedValue(ptr any) any {
	if ptr == nil {
		return nil
	}

	// Use reflection to dereference the pointer
	val := reflect.ValueOf(ptr)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}

	// Convert to any and apply type conversions
	return convertValue(val.Interface())
}

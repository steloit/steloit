package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	"brokle/internal/core/domain/billing"
)

type billableUsageRepository struct {
	db driver.Conn
}

func NewBillableUsageRepository(db driver.Conn) billing.BillableUsageRepository {
	return &billableUsageRepository{db: db}
}

func (r *billableUsageRepository) GetUsage(ctx context.Context, filter *billing.BillableUsageFilter) ([]*billing.BillableUsage, error) {
	table := "billable_usage_hourly"
	timeCol := "bucket_hour"
	if filter.Granularity == "daily" {
		table = "billable_usage_daily"
		timeCol = "bucket_date"
	}

	query := fmt.Sprintf(`
		SELECT
			organization_id,
			project_id,
			%s as bucket_time,
			sum(span_count) as span_count,
			sum(bytes_processed) as bytes_processed,
			sum(score_count) as score_count,
			toFloat64(sum(ai_provider_cost)) as ai_provider_cost,
			max(last_updated) as last_updated
		FROM %s
		WHERE organization_id = ?
			AND %s >= ?
			AND %s < ?
	`, timeCol, table, timeCol, timeCol)

	args := []any{
		filter.OrganizationID.String(),
		filter.Start,
		filter.End,
	}

	if filter.ProjectID != nil {
		query += " AND project_id = ?"
		args = append(args, filter.ProjectID.String())
	}

	query += fmt.Sprintf(" GROUP BY organization_id, project_id, %s ORDER BY %s ASC", timeCol, timeCol)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query billable usage: %w", err)
	}
	defer rows.Close()

	var result []*billing.BillableUsage
	for rows.Next() {
		var orgID, projectID string
		var bucketTime time.Time
		var spanCount, bytesProcessed, scoreCount uint64
		var aiProviderCost float64
		var lastUpdated time.Time

		if err := rows.Scan(&orgID, &projectID, &bucketTime, &spanCount, &bytesProcessed, &scoreCount, &aiProviderCost, &lastUpdated); err != nil {
			return nil, fmt.Errorf("scan billable usage row: %w", err)
		}

		parsedOrgID, _ := uuid.Parse(orgID)
		parsedProjID, _ := uuid.Parse(projectID)

		result = append(result, &billing.BillableUsage{
			OrganizationID: parsedOrgID,
			ProjectID:      parsedProjID,
			BucketTime:     bucketTime,
			SpanCount:      int64(spanCount),
			BytesProcessed: int64(bytesProcessed),
			ScoreCount:     int64(scoreCount),
			AIProviderCost: decimal.NewFromFloat(aiProviderCost),
			LastUpdated:    lastUpdated,
		})
	}

	return result, nil
}

func (r *billableUsageRepository) GetUsageSummary(ctx context.Context, filter *billing.BillableUsageFilter) (*billing.BillableUsageSummary, error) {
	// Use daily table for longer periods, hourly for recent data
	table := "billable_usage_hourly"
	timeCol := "bucket_hour"
	if filter.End.Sub(filter.Start) > 7*24*time.Hour {
		table = "billable_usage_daily"
		timeCol = "bucket_date"
	}

	query := fmt.Sprintf(`
		SELECT
			sum(span_count) as total_spans,
			sum(bytes_processed) as total_bytes,
			sum(score_count) as total_scores,
			toFloat64(sum(ai_provider_cost)) as total_ai_provider_cost
		FROM %s
		WHERE organization_id = ?
			AND %s >= ?
			AND %s < ?
	`, table, timeCol, timeCol)

	args := []any{
		filter.OrganizationID.String(),
		filter.Start,
		filter.End,
	}

	if filter.ProjectID != nil {
		query += " AND project_id = ?"
		args = append(args, filter.ProjectID.String())
	}

	var totalSpans, totalBytes, totalScores uint64
	var totalAIProviderCost float64

	row := r.db.QueryRow(ctx, query, args...)
	if err := row.Scan(&totalSpans, &totalBytes, &totalScores, &totalAIProviderCost); err != nil {
		return nil, fmt.Errorf("query usage summary: %w", err)
	}

	return &billing.BillableUsageSummary{
		OrganizationID:      filter.OrganizationID,
		ProjectID:           filter.ProjectID,
		PeriodStart:         filter.Start,
		PeriodEnd:           filter.End,
		TotalSpans:          int64(totalSpans),
		TotalBytes:          int64(totalBytes),
		TotalScores:         int64(totalScores),
		TotalAIProviderCost: decimal.NewFromFloat(totalAIProviderCost),
	}, nil
}

func (r *billableUsageRepository) GetUsageByProject(ctx context.Context, orgID uuid.UUID, start, end time.Time) ([]*billing.BillableUsageSummary, error) {
	// Use daily table for longer periods
	table := "billable_usage_hourly"
	timeCol := "bucket_hour"
	if end.Sub(start) > 7*24*time.Hour {
		table = "billable_usage_daily"
		timeCol = "bucket_date"
	}

	query := fmt.Sprintf(`
		SELECT
			project_id,
			sum(span_count) as total_spans,
			sum(bytes_processed) as total_bytes,
			sum(score_count) as total_scores,
			toFloat64(sum(ai_provider_cost)) as total_ai_provider_cost
		FROM %s
		WHERE organization_id = ?
			AND %s >= ?
			AND %s < ?
		GROUP BY project_id
		ORDER BY total_spans DESC
	`, table, timeCol, timeCol)

	rows, err := r.db.Query(ctx, query, orgID.String(), start, end)
	if err != nil {
		return nil, fmt.Errorf("query usage by project: %w", err)
	}
	defer rows.Close()

	var result []*billing.BillableUsageSummary
	for rows.Next() {
		var projectID string
		var totalSpans, totalBytes, totalScores uint64
		var totalAIProviderCost float64

		if err := rows.Scan(&projectID, &totalSpans, &totalBytes, &totalScores, &totalAIProviderCost); err != nil {
			return nil, fmt.Errorf("scan usage by project row: %w", err)
		}

		projID, _ := uuid.Parse(projectID)

		result = append(result, &billing.BillableUsageSummary{
			OrganizationID:      orgID,
			ProjectID:           &projID,
			PeriodStart:         start,
			PeriodEnd:           end,
			TotalSpans:          int64(totalSpans),
			TotalBytes:          int64(totalBytes),
			TotalScores:         int64(totalScores),
			TotalAIProviderCost: decimal.NewFromFloat(totalAIProviderCost),
		})
	}

	return result, nil
}

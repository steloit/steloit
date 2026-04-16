package observability

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// FilterPreset represents a saved filter configuration for traces or spans.
type FilterPreset struct {
	ID               uuid.UUID       `json:"id" gorm:"primaryKey;column:id;type:uuid"`
	ProjectID        uuid.UUID       `json:"project_id" gorm:"column:project_id;type:uuid;not null"`
	Name             string          `json:"name" gorm:"column:name;not null"`
	Description      *string         `json:"description,omitempty" gorm:"column:description"`
	TargetTable      string          `json:"table_name" gorm:"column:table_name;not null;default:traces"`            // "traces" or "spans"
	Filters          json.RawMessage `json:"filters" gorm:"column:filters;type:jsonb;not null" swaggertype:"object"` // Array of filter conditions
	ColumnOrder      json.RawMessage `json:"column_order,omitempty" gorm:"column:column_order;type:jsonb" swaggertype:"object"`
	ColumnVisibility json.RawMessage `json:"column_visibility,omitempty" gorm:"column:column_visibility;type:jsonb" swaggertype:"object"`
	SearchQuery      *string         `json:"search_query,omitempty" gorm:"column:search_query"`
	SearchTypes      StringArray     `json:"search_types,omitempty" gorm:"column:search_types;type:text[]"`
	IsPublic         bool            `json:"is_public" gorm:"column:is_public;not null;default:false"`
	CreatedBy        *uuid.UUID      `json:"created_by,omitempty" gorm:"column:created_by;type:uuid"`
	CreatedAt        time.Time       `json:"created_at" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt        time.Time       `json:"updated_at" gorm:"column:updated_at;autoUpdateTime"`
}

// TableName returns the database table name for FilterPreset.
func (FilterPreset) TableName() string {
	return "filter_presets"
}

// StringArray is a custom type for PostgreSQL text[] that implements Scanner and Valuer.
type StringArray []string

// Scan implements sql.Scanner for StringArray.
func (a *StringArray) Scan(value interface{}) error {
	return (*pq.StringArray)(a).Scan(value)
}

// Value implements driver.Valuer for StringArray.
func (a StringArray) Value() (driver.Value, error) {
	return pq.StringArray(a).Value()
}

// FilterCondition represents a single filter condition in a preset.
type FilterCondition struct {
	ID       string      `json:"id"`
	Column   string      `json:"column"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
	Type     string      `json:"type"` // "string", "number", "date", etc.
}

// FilterPresetTableName defines valid table names for presets.
type FilterPresetTableName string

const (
	FilterPresetTableTraces FilterPresetTableName = "traces"
	FilterPresetTableSpans  FilterPresetTableName = "spans"
)

// ValidFilterPresetTableNames returns all valid table names.
func ValidFilterPresetTableNames() []FilterPresetTableName {
	return []FilterPresetTableName{FilterPresetTableTraces, FilterPresetTableSpans}
}

// IsValidFilterPresetTableName checks if the table name is valid.
func IsValidFilterPresetTableName(name string) bool {
	switch FilterPresetTableName(name) {
	case FilterPresetTableTraces, FilterPresetTableSpans:
		return true
	}
	return false
}

// CreateFilterPresetRequest represents the request to create a filter preset.
type CreateFilterPresetRequest struct {
	Name             string          `json:"name" validate:"required,min=1,max=255"`
	Description      *string         `json:"description,omitempty" validate:"omitempty,max=1000"`
	TargetTable      string          `json:"table_name" validate:"required,oneof=traces spans"`
	Filters          json.RawMessage `json:"filters" validate:"required" swaggertype:"object"`
	ColumnOrder      json.RawMessage `json:"column_order,omitempty" swaggertype:"object"`
	ColumnVisibility json.RawMessage `json:"column_visibility,omitempty" swaggertype:"object"`
	SearchQuery      *string         `json:"search_query,omitempty" validate:"omitempty,max=500"`
	SearchTypes      []string        `json:"search_types,omitempty"`
	IsPublic         bool            `json:"is_public"`
}

// UpdateFilterPresetRequest represents the request to update a filter preset.
type UpdateFilterPresetRequest struct {
	Name             *string         `json:"name,omitempty" validate:"omitempty,min=1,max=255"`
	Description      *string         `json:"description,omitempty" validate:"omitempty,max=1000"`
	Filters          json.RawMessage `json:"filters,omitempty" swaggertype:"object"`
	ColumnOrder      json.RawMessage `json:"column_order,omitempty" swaggertype:"object"`
	ColumnVisibility json.RawMessage `json:"column_visibility,omitempty" swaggertype:"object"`
	SearchQuery      *string         `json:"search_query,omitempty" validate:"omitempty,max=500"`
	SearchTypes      []string        `json:"search_types,omitempty"`
	IsPublic         *bool           `json:"is_public,omitempty"`
}

// FilterPresetFilter represents filter options for listing presets.
type FilterPresetFilter struct {
	ProjectID   uuid.UUID  `json:"project_id"`
	TargetTable *string    `json:"table_name,omitempty"`
	CreatedBy   *uuid.UUID `json:"created_by,omitempty"`
	IsPublic    *bool      `json:"is_public,omitempty"`
	IncludeAll  bool       `json:"include_all,omitempty"` // Include both owned and public presets
	UserID      *uuid.UUID `json:"user_id,omitempty"`     // For filtering owned + public
	Limit       int        `json:"limit,omitempty"`
	Offset      int        `json:"offset,omitempty"`
}

// FilterPresetRepository defines the interface for filter preset persistence.
type FilterPresetRepository interface {
	Create(ctx context.Context, preset *FilterPreset) error
	GetByID(ctx context.Context, id uuid.UUID) (*FilterPreset, error)
	Update(ctx context.Context, preset *FilterPreset) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, filter *FilterPresetFilter) ([]*FilterPreset, error)
	Count(ctx context.Context, filter *FilterPresetFilter) (int64, error)
	ExistsByName(ctx context.Context, projectID uuid.UUID, name string, excludeID *uuid.UUID) (bool, error)
}

// Filter preset limits
const (
	FilterPresetMaxFilters      = 50
	FilterPresetMaxColumns      = 100
	FilterPresetNameMaxLength   = 255
	FilterPresetDescMaxLength   = 1000
	FilterPresetSearchMaxLength = 500
)

// ValidationError represents a validation error for filter presets.
type FilterPresetValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidateCreateFilterPresetRequest validates the create request.
func ValidateCreateFilterPresetRequest(req *CreateFilterPresetRequest) []FilterPresetValidationError {
	var errs []FilterPresetValidationError

	if req.Name == "" {
		errs = append(errs, FilterPresetValidationError{Field: "name", Message: "name is required"})
	} else if len(req.Name) > FilterPresetNameMaxLength {
		errs = append(errs, FilterPresetValidationError{Field: "name", Message: "name is too long"})
	}

	if req.Description != nil && len(*req.Description) > FilterPresetDescMaxLength {
		errs = append(errs, FilterPresetValidationError{Field: "description", Message: "description is too long"})
	}

	if !IsValidFilterPresetTableName(req.TargetTable) {
		errs = append(errs, FilterPresetValidationError{Field: "table_name", Message: "table_name must be 'traces' or 'spans'"})
	}

	if len(req.Filters) == 0 {
		errs = append(errs, FilterPresetValidationError{Field: "filters", Message: "filters is required"})
	}

	if req.SearchQuery != nil && len(*req.SearchQuery) > FilterPresetSearchMaxLength {
		errs = append(errs, FilterPresetValidationError{Field: "search_query", Message: "search_query is too long"})
	}

	return errs
}

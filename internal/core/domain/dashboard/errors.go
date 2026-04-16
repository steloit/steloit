package dashboard

import "errors"

// Domain errors for dashboard operations
var (
	// Dashboard errors
	ErrDashboardNotFound      = errors.New("dashboard not found")
	ErrDashboardAlreadyExists = errors.New("dashboard with this name already exists")
	ErrCannotDeleteDefault    = errors.New("cannot delete default dashboard while it is set as default")

	// Template errors
	ErrTemplateNotFound = errors.New("template not found")

	// Widget errors
	ErrInvalidWidgetType  = errors.New("invalid widget type")
	ErrInvalidWidgetQuery = errors.New("invalid widget query configuration")
	ErrWidgetNotFound     = errors.New("widget not found in dashboard")
	ErrDuplicateWidgetID  = errors.New("duplicate widget ID in dashboard")

	// Query errors
	ErrInvalidViewType       = errors.New("invalid view type")
	ErrInvalidMeasure        = errors.New("invalid measure for view type")
	ErrInvalidDimension      = errors.New("invalid dimension for view type")
	ErrInvalidFilterField    = errors.New("invalid filter field")
	ErrInvalidTimeRange      = errors.New("invalid time range configuration")
	ErrInvalidFilterOperator = errors.New("invalid filter operator")

	// Layout errors
	ErrInvalidLayout        = errors.New("invalid layout configuration")
	ErrLayoutWidgetMismatch = errors.New("layout widget ID does not match any widget")
	ErrLayoutOverlap        = errors.New("widgets overlap in layout")

	// General validation errors
	ErrValidationFailed = errors.New("validation failed")
	ErrInvalidProjectID = errors.New("invalid project ID")
	ErrInvalidUserID    = errors.New("invalid user ID")

	// Operation errors
	ErrUnauthorizedAccess = errors.New("unauthorized access")
)

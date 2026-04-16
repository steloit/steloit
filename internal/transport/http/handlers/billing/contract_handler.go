package billing

import (
	"log/slog"
	"time"

	"github.com/shopspring/decimal"

	"brokle/internal/config"
	"brokle/internal/core/domain/billing"
	"brokle/internal/transport/http/middleware"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/response"
	"brokle/pkg/ulid"

	"github.com/gin-gonic/gin"
)

// float64ToDecimalPtr converts a *float64 to *decimal.Decimal
func float64ToDecimalPtr(f *float64) *decimal.Decimal {
	if f == nil {
		return nil
	}
	d := decimal.NewFromFloat(*f)
	return &d
}

// float64ToDecimal converts a float64 to decimal.Decimal
func float64ToDecimal(f float64) decimal.Decimal {
	return decimal.NewFromFloat(f)
}

type ContractHandler struct {
	config          *config.Config
	logger          *slog.Logger
	contractService billing.ContractService
	pricingService  billing.PricingService
}

func NewContractHandler(
	config *config.Config,
	logger *slog.Logger,
	contractService billing.ContractService,
	pricingService billing.PricingService,
) *ContractHandler {
	return &ContractHandler{
		config:          config,
		logger:          logger,
		contractService: contractService,
		pricingService:  pricingService,
	}
}

// Request/Response DTOs

type CreateContractRequest struct {
	OrganizationID          string                      `json:"organization_id" binding:"required"`
	ContractName            string                      `json:"contract_name" binding:"required"`
	ContractNumber          string                      `json:"contract_number" binding:"required"`
	StartsAt                time.Time                   `json:"starts_at" binding:"required"` // RFC3339: 2026-01-08T10:15:00Z
	ExpiresAt               *time.Time                  `json:"expires_at,omitempty"`         // RFC3339: 2026-02-08T10:15:00Z
	MinimumCommitAmount     *float64                    `json:"minimum_commit_amount,omitempty"`
	Currency                string                      `json:"currency,omitempty"`
	AccountOwner            string                      `json:"account_owner,omitempty"`
	SalesRepEmail           string                      `json:"sales_rep_email,omitempty"`
	CustomFreeSpans         *int64                      `json:"custom_free_spans,omitempty"`
	CustomPricePer100KSpans *float64                    `json:"custom_price_per_100k_spans,omitempty"`
	CustomFreeGB            *float64                    `json:"custom_free_gb,omitempty"`
	CustomPricePerGB        *float64                    `json:"custom_price_per_gb,omitempty"`
	CustomFreeScores        *int64                      `json:"custom_free_scores,omitempty"`
	CustomPricePer1KScores  *float64                    `json:"custom_price_per_1k_scores,omitempty"`
	Notes                   string                      `json:"notes,omitempty"`
	VolumeTiers             []CreateVolumeTierRequest   `json:"volume_tiers,omitempty"`
}

type CreateVolumeTierRequest struct {
	Dimension    string   `json:"dimension" binding:"required,oneof=spans bytes scores"`
	TierMin      int64    `json:"tier_min" binding:"required,min=0"`
	TierMax      *int64   `json:"tier_max,omitempty"`
	PricePerUnit float64  `json:"price_per_unit" binding:"required,min=0"`
}

type UpdateContractRequest struct {
	ContractName            *string    `json:"contract_name,omitempty"`
	StartsAt                *time.Time `json:"starts_at,omitempty"` // RFC3339: 2026-01-08T10:15:00Z
	ExpiresAt               *time.Time `json:"expires_at,omitempty"` // RFC3339: 2026-02-08T10:15:00Z
	MinimumCommitAmount     *float64   `json:"minimum_commit_amount,omitempty"`
	AccountOwner            *string    `json:"account_owner,omitempty"`
	SalesRepEmail           *string    `json:"sales_rep_email,omitempty"`
	CustomFreeSpans         *int64     `json:"custom_free_spans,omitempty"`
	CustomPricePer100KSpans *float64   `json:"custom_price_per_100k_spans,omitempty"`
	CustomFreeGB            *float64   `json:"custom_free_gb,omitempty"`
	CustomPricePerGB        *float64   `json:"custom_price_per_gb,omitempty"`
	CustomFreeScores        *int64     `json:"custom_free_scores,omitempty"`
	CustomPricePer1KScores  *float64   `json:"custom_price_per_1k_scores,omitempty"`
	Notes                   *string    `json:"notes,omitempty"`
}

type CancelContractRequest struct {
	Reason string `json:"reason" binding:"required"`
}

type UpdateVolumeTiersRequest struct {
	Tiers []CreateVolumeTierRequest `json:"tiers" binding:"required"`
}

// CreateContract handles POST /api/v1/billing/contracts
// @Summary Create enterprise contract
// @Description Create a new enterprise contract with optional volume tiers. Timestamps must be RFC3339 format (2026-01-08T10:15:00Z). Access rule: now < expires_at. Monthly billing: Jan 8 10:15 → Feb 8 10:15 (same time next month). Minimum duration: 1 day.
// @Tags Billing - Contracts
// @Accept json
// @Produce json
// @Param contract body CreateContractRequest true "Contract details"
// @Success 201 {object} response.SuccessResponse{data=billing.Contract}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 403 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/billing/contracts [post]
func (h *ContractHandler) CreateContract(c *gin.Context) {
	var req CreateContractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	orgID, err := ulid.Parse(req.OrganizationID)
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid organization ID", "organization_id must be a valid ULID"))
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	userID, ok := middleware.GetUserIDULID(c)
	if !ok {
		response.Error(c, appErrors.NewUnauthorizedError("User context required"))
		return
	}

	// Build contract entity
	contract := &billing.Contract{
		ID:                      ulid.New(),
		OrganizationID:          orgID,
		ContractName:            req.ContractName,
		ContractNumber:          req.ContractNumber,
		StartDate:               req.StartsAt,  // API StartsAt → DB StartDate
		EndDate:                 req.ExpiresAt, // API ExpiresAt → DB EndDate
		MinimumCommitAmount:     float64ToDecimalPtr(req.MinimumCommitAmount),
		Currency:                "USD",
		AccountOwner:            req.AccountOwner,
		SalesRepEmail:           req.SalesRepEmail,
		Status:                  billing.ContractStatusDraft,
		CustomFreeSpans:         req.CustomFreeSpans,
		CustomPricePer100KSpans: float64ToDecimalPtr(req.CustomPricePer100KSpans),
		CustomFreeGB:            float64ToDecimalPtr(req.CustomFreeGB),
		CustomPricePerGB:        float64ToDecimalPtr(req.CustomPricePerGB),
		CustomFreeScores:        req.CustomFreeScores,
		CustomPricePer1KScores:  float64ToDecimalPtr(req.CustomPricePer1KScores),
		CreatedBy:               userID.String(),
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
		Notes:                   req.Notes,
	}

	if req.Currency != "" {
		contract.Currency = req.Currency
	}

	// Create contract
	if err := h.contractService.CreateContract(c.Request.Context(), contract); err != nil {
		h.logger.Error("failed to create contract",
			"error", err,
			"organization_id", orgID,
		)
		response.Error(c, err)
		return
	}

	// Add volume tiers if provided
	if len(req.VolumeTiers) > 0 {
		tiers := make([]*billing.VolumeDiscountTier, len(req.VolumeTiers))
		for i, tierReq := range req.VolumeTiers {
			tiers[i] = &billing.VolumeDiscountTier{
				ID:           ulid.New(),
				ContractID:   contract.ID,
				Dimension:    billing.TierDimension(tierReq.Dimension),
				TierMin:      tierReq.TierMin,
				TierMax:      tierReq.TierMax,
				PricePerUnit: float64ToDecimal(tierReq.PricePerUnit),
				Priority:     i,
				CreatedAt:    time.Now(),
			}
		}

		if err := h.contractService.AddVolumeTiers(c.Request.Context(), contract.ID, tiers); err != nil {
			h.logger.Error("failed to add volume tiers",
				"error", err,
				"contract_id", contract.ID,
			)
			response.Error(c, err)
			return
		}
	}

	// Reload contract with tiers
	result, err := h.contractService.GetContract(c.Request.Context(), contract.ID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, result)
}

// GetContract handles GET /api/v1/billing/contracts/:contractId
// @Summary Get contract by ID
// @Description Get contract details including volume tiers
// @Tags Billing - Contracts
// @Accept json
// @Produce json
// @Param contractId path string true "Contract ID"
// @Success 200 {object} response.SuccessResponse{data=billing.Contract}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/billing/contracts/{contractId} [get]
func (h *ContractHandler) GetContract(c *gin.Context) {
	contractID, ok := h.parseContractID(c)
	if !ok {
		return
	}

	contract, err := h.contractService.GetContract(c.Request.Context(), contractID)
	if err != nil {
		response.Error(c, err)
		return
	}

	if err := h.verifyOrgAccess(c, contract.OrganizationID); err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, contract)
}

// GetContractsByOrg handles GET /api/v1/billing/organizations/:orgId/contracts
// @Summary List organization contracts
// @Description Get all contracts for an organization
// @Tags Billing - Contracts
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Success 200 {object} response.SuccessResponse{data=[]billing.Contract}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/billing/organizations/{orgId}/contracts [get]
func (h *ContractHandler) GetContractsByOrg(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	contracts, err := h.contractService.GetContractsByOrg(c.Request.Context(), orgID)
	if err != nil {
		h.logger.Error("failed to get contracts",
			"error", err,
			"organization_id", orgID,
		)
		response.Error(c, appErrors.NewInternalError("Failed to get contracts", err))
		return
	}

	response.Success(c, contracts)
}

// UpdateContract handles PUT /api/v1/billing/contracts/:contractId
// @Summary Update contract
// @Description Update contract details (cannot change pricing after activation). Timestamps must be RFC3339 format (2026-01-08T10:15:00Z). Access rule: now < expires_at. Minimum duration: 1 day.
// @Tags Billing - Contracts
// @Accept json
// @Produce json
// @Param contractId path string true "Contract ID"
// @Param contract body UpdateContractRequest true "Contract updates"
// @Success 200 {object} response.SuccessResponse{data=billing.Contract}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/billing/contracts/{contractId} [put]
func (h *ContractHandler) UpdateContract(c *gin.Context) {
	contractID, ok := h.parseContractID(c)
	if !ok {
		return
	}

	var req UpdateContractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	// Get existing contract
	contract, err := h.contractService.GetContract(c.Request.Context(), contractID)
	if err != nil {
		response.Error(c, err)
		return
	}

	if err := h.verifyOrgAccess(c, contract.OrganizationID); err != nil {
		response.Error(c, err)
		return
	}

	// Apply updates
	if req.ContractName != nil {
		contract.ContractName = *req.ContractName
	}
	if req.StartsAt != nil {
		contract.StartDate = *req.StartsAt
	}
	if req.ExpiresAt != nil {
		contract.EndDate = req.ExpiresAt
	}
	if req.MinimumCommitAmount != nil {
		contract.MinimumCommitAmount = float64ToDecimalPtr(req.MinimumCommitAmount)
	}
	if req.AccountOwner != nil {
		contract.AccountOwner = *req.AccountOwner
	}
	if req.SalesRepEmail != nil {
		contract.SalesRepEmail = *req.SalesRepEmail
	}
	if req.CustomFreeSpans != nil {
		contract.CustomFreeSpans = req.CustomFreeSpans
	}
	if req.CustomPricePer100KSpans != nil {
		contract.CustomPricePer100KSpans = float64ToDecimalPtr(req.CustomPricePer100KSpans)
	}
	if req.CustomFreeGB != nil {
		contract.CustomFreeGB = float64ToDecimalPtr(req.CustomFreeGB)
	}
	if req.CustomPricePerGB != nil {
		contract.CustomPricePerGB = float64ToDecimalPtr(req.CustomPricePerGB)
	}
	if req.CustomFreeScores != nil {
		contract.CustomFreeScores = req.CustomFreeScores
	}
	if req.CustomPricePer1KScores != nil {
		contract.CustomPricePer1KScores = float64ToDecimalPtr(req.CustomPricePer1KScores)
	}
	if req.Notes != nil {
		contract.Notes = *req.Notes
	}

	contract.UpdatedAt = time.Now()

	if err := h.contractService.UpdateContract(c.Request.Context(), contract); err != nil {
		h.logger.Error("failed to update contract",
			"error", err,
			"contract_id", contractID,
		)
		response.Error(c, err)
		return
	}

	response.Success(c, contract)
}

// ActivateContract handles PUT /api/v1/billing/contracts/:contractId/activate
// @Summary Activate contract
// @Description Activate a draft contract (expires any existing active contract)
// @Tags Billing - Contracts
// @Accept json
// @Produce json
// @Param contractId path string true "Contract ID"
// @Success 200 {object} response.SuccessResponse{data=map[string]string}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/billing/contracts/{contractId}/activate [put]
func (h *ContractHandler) ActivateContract(c *gin.Context) {
	contractID, ok := h.parseContractID(c)
	if !ok {
		return
	}

	contract, err := h.contractService.GetContract(c.Request.Context(), contractID)
	if err != nil {
		response.Error(c, err)
		return
	}

	if err := h.verifyOrgAccess(c, contract.OrganizationID); err != nil {
		response.Error(c, err)
		return
	}

	userID, ok := middleware.GetUserIDULID(c)
	if !ok {
		response.Error(c, appErrors.NewUnauthorizedError("User context required"))
		return
	}

	if err := h.contractService.ActivateContract(c.Request.Context(), contractID, userID); err != nil {
		h.logger.Error("failed to activate contract",
			"error", err,
			"contract_id", contractID,
		)
		response.Error(c, err)
		return
	}

	response.Success(c, map[string]string{
		"message": "Contract activated successfully",
		"status":  "active",
	})
}

// CancelContract handles DELETE /api/v1/billing/contracts/:contractId
// @Summary Cancel contract
// @Description Cancel an active contract with reason
// @Tags Billing - Contracts
// @Accept json
// @Produce json
// @Param contractId path string true "Contract ID"
// @Param request body CancelContractRequest true "Cancellation reason"
// @Success 200 {object} response.SuccessResponse{data=map[string]string}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/billing/contracts/{contractId} [delete]
func (h *ContractHandler) CancelContract(c *gin.Context) {
	contractID, ok := h.parseContractID(c)
	if !ok {
		return
	}

	var req CancelContractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	contract, err := h.contractService.GetContract(c.Request.Context(), contractID)
	if err != nil {
		response.Error(c, err)
		return
	}

	if err := h.verifyOrgAccess(c, contract.OrganizationID); err != nil {
		response.Error(c, err)
		return
	}

	userID, ok := middleware.GetUserIDULID(c)
	if !ok {
		response.Error(c, appErrors.NewUnauthorizedError("User context required"))
		return
	}

	if err := h.contractService.CancelContract(c.Request.Context(), contractID, req.Reason, userID); err != nil {
		h.logger.Error("failed to cancel contract",
			"error", err,
			"contract_id", contractID,
		)
		response.Error(c, err)
		return
	}

	response.Success(c, map[string]string{
		"message": "Contract cancelled successfully",
		"status":  "cancelled",
	})
}

// UpdateVolumeTiers handles PUT /api/v1/billing/contracts/:contractId/tiers
// @Summary Update volume discount tiers
// @Description Replace all volume discount tiers for a contract
// @Tags Billing - Contracts
// @Accept json
// @Produce json
// @Param contractId path string true "Contract ID"
// @Param tiers body UpdateVolumeTiersRequest true "Volume tiers"
// @Success 200 {object} response.SuccessResponse{data=map[string]string}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/billing/contracts/{contractId}/tiers [put]
func (h *ContractHandler) UpdateVolumeTiers(c *gin.Context) {
	contractID, ok := h.parseContractID(c)
	if !ok {
		return
	}

	var req UpdateVolumeTiersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid request body", err.Error()))
		return
	}

	contract, err := h.contractService.GetContract(c.Request.Context(), contractID)
	if err != nil {
		response.Error(c, err)
		return
	}

	if err := h.verifyOrgAccess(c, contract.OrganizationID); err != nil {
		response.Error(c, err)
		return
	}

	// Build tier entities
	tiers := make([]*billing.VolumeDiscountTier, len(req.Tiers))
	for i, tierReq := range req.Tiers {
		tiers[i] = &billing.VolumeDiscountTier{
			ID:           ulid.New(),
			ContractID:   contractID,
			Dimension:    billing.TierDimension(tierReq.Dimension),
			TierMin:      tierReq.TierMin,
			TierMax:      tierReq.TierMax,
			PricePerUnit: float64ToDecimal(tierReq.PricePerUnit),
			Priority:     i,
			CreatedAt:    time.Now(),
		}
	}

	if err := h.contractService.UpdateVolumeTiers(c.Request.Context(), contractID, tiers); err != nil {
		h.logger.Error("failed to update volume tiers",
			"error", err,
			"contract_id", contractID,
		)
		response.Error(c, err)
		return
	}

	response.Success(c, map[string]interface{}{
		"message":     "Volume tiers updated successfully",
		"tiers_count": len(tiers),
	})
}

// GetContractHistory handles GET /api/v1/billing/contracts/:contractId/history
// @Summary Get contract audit history
// @Description Get full audit trail of contract changes
// @Tags Billing - Contracts
// @Accept json
// @Produce json
// @Param contractId path string true "Contract ID"
// @Success 200 {object} response.SuccessResponse{data=[]billing.ContractHistory}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/billing/contracts/{contractId}/history [get]
func (h *ContractHandler) GetContractHistory(c *gin.Context) {
	contractID, ok := h.parseContractID(c)
	if !ok {
		return
	}

	contract, err := h.contractService.GetContract(c.Request.Context(), contractID)
	if err != nil {
		response.Error(c, err)
		return
	}

	if err := h.verifyOrgAccess(c, contract.OrganizationID); err != nil {
		response.Error(c, err)
		return
	}

	history, err := h.contractService.GetContractHistory(c.Request.Context(), contractID)
	if err != nil {
		h.logger.Error("failed to get contract history",
			"error", err,
			"contract_id", contractID,
		)
		response.Error(c, appErrors.NewInternalError("Failed to get contract history", err))
		return
	}

	response.Success(c, history)
}

// GetEffectivePricing handles GET /api/v1/billing/organizations/:orgId/effective-pricing
// @Summary Get effective pricing for organization
// @Description Get resolved pricing (contract overrides > plan defaults)
// @Tags Billing - Contracts
// @Accept json
// @Produce json
// @Param orgId path string true "Organization ID"
// @Success 200 {object} response.SuccessResponse{data=billing.EffectivePricing}
// @Failure 400 {object} response.ErrorResponse
// @Failure 401 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/billing/organizations/{orgId}/effective-pricing [get]
func (h *ContractHandler) GetEffectivePricing(c *gin.Context) {
	orgID, ok := h.parseOrgID(c)
	if !ok {
		return
	}

	if err := h.verifyOrgAccess(c, orgID); err != nil {
		response.Error(c, err)
		return
	}

	effectivePricing, err := h.pricingService.GetEffectivePricing(c.Request.Context(), orgID)
	if err != nil {
		h.logger.Error("failed to get effective pricing",
			"error", err,
			"organization_id", orgID,
		)
		response.Error(c, appErrors.NewInternalError("Failed to get effective pricing", err))
		return
	}

	response.Success(c, effectivePricing)
}

// Helper methods

func (h *ContractHandler) parseContractID(c *gin.Context) (ulid.ULID, bool) {
	contractID, err := ulid.Parse(c.Param("contractId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid contract ID", "contractId must be a valid ULID"))
		return ulid.ULID{}, false
	}
	return contractID, true
}

func (h *ContractHandler) parseOrgID(c *gin.Context) (ulid.ULID, bool) {
	orgID, err := ulid.Parse(c.Param("orgId"))
	if err != nil {
		response.Error(c, appErrors.NewValidationError("Invalid organization ID", "orgId must be a valid ULID"))
		return ulid.ULID{}, false
	}
	return orgID, true
}

func (h *ContractHandler) verifyOrgAccess(c *gin.Context, orgID ulid.ULID) error {
	userOrgID := middleware.ResolveOrganizationID(c)
	if userOrgID == nil || userOrgID.IsZero() {
		return appErrors.NewUnauthorizedError("Organization context required")
	}

	if *userOrgID != orgID {
		return appErrors.NewForbiddenError("Access denied to this organization")
	}

	return nil
}

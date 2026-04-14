package website

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"brokle/internal/core/domain/website"
	"brokle/pkg/response"
)

type Handler struct {
	websiteService website.WebsiteService
	logger         *slog.Logger
}

func NewHandler(logger *slog.Logger, websiteService website.WebsiteService) *Handler {
	return &Handler{
		websiteService: websiteService,
		logger:         logger,
	}
}

// SubmitContactForm handles contact form submissions from the marketing website.
func (h *Handler) SubmitContactForm(c *gin.Context) {
	var req website.CreateContactSubmissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, 400, "VALIDATION_ERROR", "Invalid request body", err.Error())
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.Request.UserAgent()

	if err := h.websiteService.SubmitContactForm(c.Request.Context(), &req, ipAddress, userAgent); err != nil {
		response.Error(c, err)
		return
	}

	response.Created(c, map[string]string{
		"message": "Thank you for your message. We'll get back to you soon.",
	})
}

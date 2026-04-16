package billing

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
	"brokle/pkg/uid"
)

// InvoiceGenerator handles invoice generation and management
type InvoiceGenerator struct {
	logger *slog.Logger
	config *BillingConfig
}

// Invoice represents a generated invoice

// NewInvoiceGenerator creates a new invoice generator instance
func NewInvoiceGenerator(logger *slog.Logger, config *BillingConfig) *InvoiceGenerator {
	return &InvoiceGenerator{
		logger: logger,
		config: config,
	}
}

// GenerateInvoice creates an invoice from a billing summary
func (g *InvoiceGenerator) GenerateInvoice(
	ctx context.Context,
	summary *billingDomain.BillingSummary,
	organizationName string,
	billingAddress *billingDomain.BillingAddress,
) (*billingDomain.Invoice, error) {

	if summary.NetCost.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("cannot generate invoice for zero or negative amount: %s", summary.NetCost)
	}

	invoice := &billingDomain.Invoice{
		ID:               uid.New(),
		InvoiceNumber:    g.generateInvoiceNumber(summary.OrganizationID, summary.PeriodStart),
		OrganizationID:   summary.OrganizationID,
		OrganizationName: organizationName,
		BillingAddress:   billingAddress,
		Period:           summary.Period,
		PeriodStart:      summary.PeriodStart,
		PeriodEnd:        summary.PeriodEnd,
		IssueDate:        time.Now(),
		DueDate:          time.Now().Add(g.config.PaymentGracePeriod),
		Currency:         summary.Currency,
		Status:           billingDomain.InvoiceStatusDraft,
		PaymentTerms:     fmt.Sprintf("Net %d days", int(g.config.PaymentGracePeriod.Hours()/24)),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Generate line items from usage breakdown
	lineItems := g.generateLineItems(summary)
	invoice.LineItems = lineItems

	// Calculate totals
	subtotal := decimal.Zero
	for _, item := range lineItems {
		subtotal = subtotal.Add(item.Amount)
	}

	invoice.Subtotal = subtotal
	invoice.DiscountAmount = summary.Discounts

	// Apply tax if configured
	taxConfig := g.getTaxConfiguration(billingAddress)
	if taxConfig != nil {
		taxableAmount := subtotal.Sub(invoice.DiscountAmount)
		invoice.TaxAmount = taxableAmount.Mul(taxConfig.TaxRate)
	}

	invoice.TotalAmount = invoice.Subtotal.Sub(invoice.DiscountAmount).Add(invoice.TaxAmount)

	// Add metadata
	invoice.Metadata = map[string]interface{}{
		"total_requests":     summary.TotalRequests,
		"total_tokens":       summary.TotalTokens,
		"provider_breakdown": summary.ProviderBreakdown,
		"model_breakdown":    summary.ModelBreakdown,
		"billing_period":     summary.Period,
	}

	g.logger.Info("Generated invoice", "invoice_id", invoice.ID, "invoice_number", invoice.InvoiceNumber, "organization_id", invoice.OrganizationID, "total_amount", invoice.TotalAmount, "currency", invoice.Currency)

	return invoice, nil
}

// GenerateInvoiceHTML generates HTML representation of an invoice
func (g *InvoiceGenerator) GenerateInvoiceHTML(ctx context.Context, invoice *billingDomain.Invoice) (string, error) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Invoice {{.InvoiceNumber}}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; color: #333; }
        .header { border-bottom: 2px solid #007acc; padding-bottom: 20px; margin-bottom: 30px; }
        .company-name { font-size: 28px; font-weight: bold; color: #007acc; }
        .invoice-title { font-size: 24px; margin: 20px 0; }
        .invoice-details { margin: 20px 0; }
        .billing-info { display: flex; justify-content: space-between; margin: 30px 0; }
        .billing-address { max-width: 300px; }
        .invoice-table { width: 100%; border-collapse: collapse; margin: 30px 0; }
        .invoice-table th, .invoice-table td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        .invoice-table th { background-color: #f8f9fa; font-weight: bold; }
        .amount { text-align: right; }
        .totals { max-width: 400px; margin-left: auto; margin-top: 30px; }
        .totals table { width: 100%; }
        .totals .total-row { font-weight: bold; font-size: 18px; border-top: 2px solid #333; }
        .payment-info { margin-top: 40px; padding: 20px; background-color: #f8f9fa; border-radius: 5px; }
        .footer { margin-top: 40px; font-size: 12px; color: #666; }
    </style>
</head>
<body>
    <div class="header">
        <div class="company-name">Brokle</div>
        <div>The Open-Source AI Control Plane</div>
    </div>

    <div class="invoice-title">Invoice {{.InvoiceNumber}}</div>

    <div class="invoice-details">
        <strong>Issue Date:</strong> {{.IssueDate.Format "January 2, 2006"}}<br>
        <strong>Due Date:</strong> {{.DueDate.Format "January 2, 2006"}}<br>
        <strong>Period:</strong> {{.PeriodStart.Format "January 2, 2006"}} - {{.PeriodEnd.Format "January 2, 2006"}}
    </div>

    <div class="billing-info">
        <div>
            <strong>Bill To:</strong><br>
            {{.OrganizationName}}<br>
            {{if .BillingAddress}}
                {{.BillingAddress.Company}}<br>
                {{.BillingAddress.Address1}}<br>
                {{if .BillingAddress.Address2}}{{.BillingAddress.Address2}}<br>{{end}}
                {{.BillingAddress.City}}, {{.BillingAddress.State}} {{.BillingAddress.PostalCode}}<br>
                {{.BillingAddress.Country}}<br>
                {{if .BillingAddress.TaxID}}<strong>Tax ID:</strong> {{.BillingAddress.TaxID}}{{end}}
            {{end}}
        </div>
        <div>
            <strong>From:</strong><br>
            Brokle Inc.<br>
            123 AI Boulevard<br>
            San Francisco, CA 94105<br>
            United States
        </div>
    </div>

    <table class="invoice-table">
        <thead>
            <tr>
                <th>Description</th>
                <th>Quantity</th>
                <th>Unit Price</th>
                <th class="amount">Amount</th>
            </tr>
        </thead>
        <tbody>
            {{range .LineItems}}
            <tr>
                <td>
                    {{.Description}}
                    {{if .ProviderName}}<br><small>Provider: {{.ProviderName}}</small>{{end}}
                    {{if .ModelName}}<br><small>Model: {{.ModelName}}</small>{{end}}
                </td>
                <td>{{printf "%.0f" .Quantity}}</td>
                <td>${{printf "%.4f" .UnitPrice}}</td>
                <td class="amount">${{printf "%.2f" .Amount}}</td>
            </tr>
            {{end}}
        </tbody>
    </table>

    <div class="totals">
        <table>
            <tr>
                <td><strong>Subtotal:</strong></td>
                <td class="amount">${{printf "%.2f" .Subtotal}}</td>
            </tr>
            {{if gt .DiscountAmount 0}}
            <tr>
                <td><strong>Discount:</strong></td>
                <td class="amount">-${{printf "%.2f" .DiscountAmount}}</td>
            </tr>
            {{end}}
            {{if gt .TaxAmount 0}}
            <tr>
                <td><strong>Tax:</strong></td>
                <td class="amount">${{printf "%.2f" .TaxAmount}}</td>
            </tr>
            {{end}}
            <tr class="total-row">
                <td><strong>Total:</strong></td>
                <td class="amount"><strong>${{printf "%.2f" .TotalAmount}} {{.Currency}}</strong></td>
            </tr>
        </table>
    </div>

    <div class="payment-info">
        <strong>Payment Terms:</strong> {{.PaymentTerms}}<br>
        <strong>Status:</strong> {{.Status}}<br>
        {{if .Notes}}<strong>Notes:</strong> {{.Notes}}<br>{{end}}
    </div>

    <div class="footer">
        <p>Thank you for using Brokle! For questions about this invoice, please contact support@brokle.com</p>
        <p>This invoice was generated automatically on {{.CreatedAt.Format "January 2, 2006 at 3:04 PM MST"}}</p>
    </div>
</body>
</html>
`

	t, err := template.New("invoice").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse invoice template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, invoice); err != nil {
		return "", fmt.Errorf("failed to execute invoice template: %w", err)
	}

	return buf.String(), nil
}

// MarkInvoiceAsPaid marks an invoice as paid
func (g *InvoiceGenerator) MarkInvoiceAsPaid(ctx context.Context, invoice *billingDomain.Invoice, paidAt time.Time) error {
	if invoice.Status == billingDomain.InvoiceStatusPaid {
		return fmt.Errorf("invoice %s is already marked as paid", invoice.InvoiceNumber)
	}

	invoice.Status = billingDomain.InvoiceStatusPaid
	invoice.PaidAt = &paidAt
	invoice.UpdatedAt = time.Now()

	g.logger.Info("Invoice marked as paid", "invoice_id", invoice.ID, "invoice_number", invoice.InvoiceNumber, "organization_id", invoice.OrganizationID, "paid_at", paidAt)

	return nil
}

// MarkInvoiceAsOverdue marks an invoice as overdue
func (g *InvoiceGenerator) MarkInvoiceAsOverdue(ctx context.Context, invoice *billingDomain.Invoice) error {
	if invoice.Status == billingDomain.InvoiceStatusPaid {
		return fmt.Errorf("cannot mark paid invoice %s as overdue", invoice.InvoiceNumber)
	}

	invoice.Status = billingDomain.InvoiceStatusOverdue
	invoice.UpdatedAt = time.Now()

	g.logger.Warn("Invoice marked as overdue", "invoice_id", invoice.ID, "invoice_number", invoice.InvoiceNumber, "organization_id", invoice.OrganizationID, "due_date", invoice.DueDate)

	return nil
}

// CancelInvoice cancels an invoice
func (g *InvoiceGenerator) CancelInvoice(ctx context.Context, invoice *billingDomain.Invoice, reason string) error {
	if invoice.Status == billingDomain.InvoiceStatusPaid {
		return fmt.Errorf("cannot cancel paid invoice %s", invoice.InvoiceNumber)
	}

	invoice.Status = billingDomain.InvoiceStatusCancelled
	invoice.UpdatedAt = time.Now()

	if invoice.Metadata == nil {
		invoice.Metadata = make(map[string]interface{})
	}
	invoice.Metadata["cancellation_reason"] = reason
	invoice.Metadata["cancelled_at"] = time.Now()

	g.logger.Info("Invoice cancelled", "invoice_id", invoice.ID, "invoice_number", invoice.InvoiceNumber, "organization_id", invoice.OrganizationID, "reason", reason)

	return nil
}

// GetInvoiceSummary generates a summary of multiple invoices
func (g *InvoiceGenerator) GetInvoiceSummary(ctx context.Context, invoices []*billingDomain.Invoice) *InvoiceSummary {
	summary := &InvoiceSummary{
		TotalInvoices:     len(invoices),
		StatusCounts:      make(map[billingDomain.InvoiceStatus]int),
		TotalAmount:       decimal.Zero,
		PaidAmount:        decimal.Zero,
		OutstandingAmount: decimal.Zero,
		OverdueAmount:     decimal.Zero,
	}

	for _, invoice := range invoices {
		// Count by status
		summary.StatusCounts[invoice.Status]++

		// Calculate amounts
		summary.TotalAmount = summary.TotalAmount.Add(invoice.TotalAmount)

		switch invoice.Status {
		case billingDomain.InvoiceStatusPaid:
			summary.PaidAmount = summary.PaidAmount.Add(invoice.TotalAmount)
		case billingDomain.InvoiceStatusOverdue:
			summary.OverdueAmount = summary.OverdueAmount.Add(invoice.TotalAmount)
			summary.OutstandingAmount = summary.OutstandingAmount.Add(invoice.TotalAmount)
		case billingDomain.InvoiceStatusSent, billingDomain.InvoiceStatusDraft:
			summary.OutstandingAmount = summary.OutstandingAmount.Add(invoice.TotalAmount)
		}

		// Track earliest and latest dates
		if summary.EarliestDate == nil || invoice.IssueDate.Before(*summary.EarliestDate) {
			summary.EarliestDate = &invoice.IssueDate
		}
		if summary.LatestDate == nil || invoice.IssueDate.After(*summary.LatestDate) {
			summary.LatestDate = &invoice.IssueDate
		}
	}

	return summary
}

// InvoiceSummary represents a summary of multiple invoices
type InvoiceSummary struct {
	StatusCounts      map[billingDomain.InvoiceStatus]int `json:"status_counts"`
	EarliestDate      *time.Time                          `json:"earliest_date,omitempty"`
	LatestDate        *time.Time                          `json:"latest_date,omitempty"`
	TotalInvoices     int                                 `json:"total_invoices"`
	TotalAmount       decimal.Decimal                     `json:"total_amount"`
	PaidAmount        decimal.Decimal                     `json:"paid_amount"`
	OutstandingAmount decimal.Decimal                     `json:"outstanding_amount"`
	OverdueAmount     decimal.Decimal                     `json:"overdue_amount"`
}

// Internal methods

func (g *InvoiceGenerator) generateInvoiceNumber(orgID uuid.UUID, periodStart time.Time) string {
	// Format: BRKL-YYYY-MM-{ORG_SHORT}-{SEQUENCE}
	orgShort := orgID.String()[:8] // First 8 characters of org ID
	yearMonth := periodStart.Format("2006-01")

	// In a real implementation, you'd want to get the next sequence number from the database
	sequence := "001"

	return fmt.Sprintf("BRKL-%s-%s-%s", yearMonth, orgShort, sequence)
}

func (g *InvoiceGenerator) generateLineItems(summary *billingDomain.BillingSummary) []billingDomain.InvoiceLineItem {
	var lineItems []billingDomain.InvoiceLineItem

	// Create line items based on provider breakdown
	for providerKey, amountInterface := range summary.ProviderBreakdown {
		amount, ok := amountInterface.(decimal.Decimal)
		if !ok {
			continue
		}
		if amount.GreaterThan(decimal.Zero) {
			lineItem := billingDomain.InvoiceLineItem{
				ID:          uid.New(),
				Description: "AI API Usage - Provider " + providerKey,
				Quantity:    decimal.NewFromInt(1),
				UnitPrice:   amount,
				Amount:      amount,
			}

			// In a real implementation, you'd look up provider details
			lineItem.ProviderName = "Provider " + providerKey[:8]

			lineItems = append(lineItems, lineItem)
		}
	}

	// If no provider breakdown, create a single line item
	if len(lineItems) == 0 {
		quantity := decimal.NewFromInt(int64(summary.TotalRequests))
		unitPrice := decimal.Zero
		if summary.TotalRequests > 0 {
			unitPrice = summary.TotalCost.Div(quantity)
		}
		lineItems = append(lineItems, billingDomain.InvoiceLineItem{
			ID:          uid.New(),
			Description: "AI API Usage - " + summary.Period,
			Quantity:    quantity,
			UnitPrice:   unitPrice,
			Amount:      summary.TotalCost,
		})
	}

	return lineItems
}

func (g *InvoiceGenerator) getTaxConfiguration(billingAddress *billingDomain.BillingAddress) *billingDomain.TaxConfiguration {
	if billingAddress == nil {
		return nil
	}

	// Simple tax configuration based on country
	// In a real implementation, this would be more sophisticated
	taxConfigs := map[string]*billingDomain.TaxConfiguration{
		"US": {
			TaxRate:     decimal.NewFromFloat(0.08), // 8% sales tax
			TaxName:     "Sales Tax",
			IsInclusive: false,
		},
		"UK": {
			TaxRate:     decimal.NewFromFloat(0.20), // 20% VAT
			TaxName:     "VAT",
			IsInclusive: false,
		},
		"CA": {
			TaxRate:     decimal.NewFromFloat(0.13), // 13% HST (varies by province)
			TaxName:     "HST",
			IsInclusive: false,
		},
	}

	return taxConfigs[billingAddress.Country]
}

// Health check
func (g *InvoiceGenerator) GetHealth() map[string]interface{} {
	return map[string]interface{}{
		"service": "invoice_generator",
		"status":  "healthy",
		"config": map[string]interface{}{
			"default_currency":     g.config.DefaultCurrency,
			"payment_grace_period": g.config.PaymentGracePeriod.String(),
			"invoice_generation":   g.config.InvoiceGeneration,
		},
	}
}

// Additional utility methods for invoice management

// IsOverdue checks if an invoice is overdue
func (g *InvoiceGenerator) IsOverdue(invoice *billingDomain.Invoice) bool {
	return invoice.Status != billingDomain.InvoiceStatusPaid &&
		invoice.Status != billingDomain.InvoiceStatusCancelled &&
		time.Now().After(invoice.DueDate)
}

// CalculateLateFee calculates late fee for an overdue invoice
func (g *InvoiceGenerator) CalculateLateFee(invoice *billingDomain.Invoice, lateFeeRate float64) decimal.Decimal {
	if !g.IsOverdue(invoice) {
		return decimal.Zero
	}

	daysOverdue := int(time.Since(invoice.DueDate).Hours() / 24)
	if daysOverdue <= 0 {
		return decimal.Zero
	}

	return invoice.TotalAmount.Mul(decimal.NewFromFloat(lateFeeRate)).Mul(decimal.NewFromInt(int64(daysOverdue))).Div(decimal.NewFromInt(365))
}

// GeneratePaymentReminder generates text for a payment reminder
func (g *InvoiceGenerator) GeneratePaymentReminder(invoice *billingDomain.Invoice) string {
	daysOverdue := int(time.Since(invoice.DueDate).Hours() / 24)

	var message string
	if daysOverdue <= 0 {
		daysToDue := int(time.Until(invoice.DueDate).Hours() / 24)
		message = fmt.Sprintf("Your invoice %s for $%s is due in %d days.",
			invoice.InvoiceNumber, invoice.TotalAmount.StringFixed(2), daysToDue)
	} else {
		message = fmt.Sprintf("Your invoice %s for $%s is %d days overdue. Please remit payment immediately.",
			invoice.InvoiceNumber, invoice.TotalAmount.StringFixed(2), daysOverdue)
	}

	return message
}

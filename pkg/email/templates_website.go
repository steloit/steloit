package email

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
)

// ContactNotificationEmailParams contains parameters for contact form notification emails.
type ContactNotificationEmailParams struct {
	Name        string
	Email       string
	Company     string
	Subject     string
	Message     string
	InquiryType string
	IPAddress   string
	AppName     string
}

// BuildContactNotificationEmail generates the HTML email for a contact form notification.
func BuildContactNotificationEmail(params ContactNotificationEmailParams) (htmlContent, text string, err error) {
	funcMap := template.FuncMap{
		"urlQueryEscape": url.QueryEscape,
	}
	htmlTmpl := template.Must(template.New("contact_html").Funcs(funcMap).Parse(contactNotificationHTMLTemplate))
	textTmpl := template.Must(template.New("contact_text").Parse(contactNotificationTextTemplate))

	if params.AppName == "" {
		params.AppName = "Brokle"
	}

	var htmlBuf bytes.Buffer
	if err := htmlTmpl.Execute(&htmlBuf, params); err != nil {
		return "", "", fmt.Errorf("failed to generate HTML email: %w", err)
	}

	var textBuf bytes.Buffer
	if err := textTmpl.Execute(&textBuf, params); err != nil {
		return "", "", fmt.Errorf("failed to generate text email: %w", err)
	}

	return htmlBuf.String(), textBuf.String(), nil
}

const contactNotificationHTMLTemplate = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>New Contact Form Submission</title>
</head>
<body style="margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif; background-color: #f4f4f5;">
  <table role="presentation" style="width: 100%; border-collapse: collapse;">
    <tr>
      <td align="center" style="padding: 40px 0;">
        <table role="presentation" style="width: 100%; max-width: 600px; border-collapse: collapse; background-color: #ffffff; border-radius: 8px; box-shadow: 0 1px 3px rgba(0,0,0,0.1);">
          <!-- Header -->
          <tr>
            <td style="padding: 32px 40px 16px 40px; background-color: #18181b; border-radius: 8px 8px 0 0;">
              <h1 style="margin: 0; font-size: 20px; font-weight: 600; color: #ffffff;">
                New Contact Form Submission
              </h1>
            </td>
          </tr>

          <!-- Body -->
          <tr>
            <td style="padding: 24px 40px;">
              <table role="presentation" style="width: 100%; border-collapse: collapse;">
                <tr>
                  <td style="padding: 8px 0;">
                    <p style="margin: 0; font-size: 13px; color: #71717a; text-transform: uppercase; letter-spacing: 0.05em;">From</p>
                    <p style="margin: 4px 0 0 0; font-size: 16px; color: #18181b;">{{.Name}} &lt;{{.Email}}&gt;</p>
                  </td>
                </tr>
                {{if .Company}}
                <tr>
                  <td style="padding: 8px 0;">
                    <p style="margin: 0; font-size: 13px; color: #71717a; text-transform: uppercase; letter-spacing: 0.05em;">Company</p>
                    <p style="margin: 4px 0 0 0; font-size: 16px; color: #18181b;">{{.Company}}</p>
                  </td>
                </tr>
                {{end}}
                {{if .InquiryType}}
                <tr>
                  <td style="padding: 8px 0;">
                    <p style="margin: 0; font-size: 13px; color: #71717a; text-transform: uppercase; letter-spacing: 0.05em;">Inquiry Type</p>
                    <p style="margin: 4px 0 0 0; font-size: 16px; color: #18181b;">{{.InquiryType}}</p>
                  </td>
                </tr>
                {{end}}
                <tr>
                  <td style="padding: 8px 0;">
                    <p style="margin: 0; font-size: 13px; color: #71717a; text-transform: uppercase; letter-spacing: 0.05em;">Subject</p>
                    <p style="margin: 4px 0 0 0; font-size: 16px; color: #18181b; font-weight: 500;">{{.Subject}}</p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Message -->
          <tr>
            <td style="padding: 0 40px 24px 40px;">
              <table role="presentation" style="width: 100%; border-collapse: collapse;">
                <tr>
                  <td style="padding: 16px; background-color: #f4f4f5; border-radius: 6px; border-left: 4px solid #3b82f6;">
                    <p style="margin: 0; font-size: 15px; line-height: 24px; color: #3f3f46; white-space: pre-wrap;">{{.Message}}</p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Reply CTA -->
          <tr>
            <td style="padding: 0 40px 24px 40px;">
              <table role="presentation" style="width: 100%; border-collapse: collapse;">
                <tr>
                  <td align="center">
                    <a href="mailto:{{.Email}}?subject={{urlQueryEscape (printf "Re: %s" .Subject)}}" style="display: inline-block; padding: 12px 28px; background-color: #18181b; color: #ffffff; text-decoration: none; font-size: 14px; font-weight: 500; border-radius: 6px;">
                      Reply to {{.Name}}
                    </a>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Divider -->
          <tr>
            <td style="padding: 0 40px;">
              <hr style="border: none; border-top: 1px solid #e4e4e7; margin: 0;">
            </td>
          </tr>

          <!-- Footer -->
          <tr>
            <td style="padding: 16px 40px 32px 40px;">
              <p style="margin: 0; font-size: 12px; line-height: 18px; color: #a1a1aa; text-align: center;">
                {{if .IPAddress}}IP: {{.IPAddress}} · {{end}}Sent via {{.AppName}} contact form
              </p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`

const contactNotificationTextTemplate = `New Contact Form Submission

From: {{.Name}} <{{.Email}}>
{{if .Company}}Company: {{.Company}}
{{end}}{{if .InquiryType}}Inquiry Type: {{.InquiryType}}
{{end}}Subject: {{.Subject}}

Message:
{{.Message}}

---
{{if .IPAddress}}IP: {{.IPAddress}} | {{end}}Sent via {{.AppName}} contact form`

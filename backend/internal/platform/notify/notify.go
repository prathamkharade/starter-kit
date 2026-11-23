// Package notify sends email and SMS through pluggable providers chosen by
// env vars (EMAIL_PROVIDER, SMS_PROVIDER). Sending always happens in the
// background worker (job kinds "email.send" / "sms.send") so API latency and
// provider outages never block users.
//
// Providers:
//
//	email: console (dev log) | smtp (works with SES, Brevo, Resend, Postmark,
//	       Mailgun, Zoho, Mailpit… — every serious provider offers SMTP)
//	sms:   none | console | twilio | http (generic JSON webhook: MSG91, Vonage,
//	       Infobip, WhatsApp gateways… configure URL/header/body template)
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	htmltpl "html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/wneessen/go-mail"

	"app/internal/config"
)

type Email struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

type EmailSender interface {
	Send(ctx context.Context, e Email) error
}

type SMSSender interface {
	Send(ctx context.Context, to, body string) error
}

// ---- constructors -----------------------------------------------------------

func NewEmailSender(cfg *config.Config, log *slog.Logger) (EmailSender, error) {
	switch cfg.EmailProvider {
	case "console":
		return consoleEmail{log}, nil
	case "smtp":
		return newSMTP(cfg)
	default:
		return nil, fmt.Errorf("unknown EMAIL_PROVIDER %q", cfg.EmailProvider)
	}
}

func NewSMSSender(cfg *config.Config, log *slog.Logger) (SMSSender, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	switch cfg.SMSProvider {
	case "none", "":
		return noSMS{}, nil
	case "console":
		return consoleSMS{log}, nil
	case "twilio":
		return twilio{cfg.TwilioSID, cfg.TwilioToken, cfg.TwilioFrom, client}, nil
	case "http":
		return httpSMS{cfg.SMSHTTPURL, cfg.SMSHTTPAuthHeader, cfg.SMSHTTPBodyTemplate, client}, nil
	default:
		return nil, fmt.Errorf("unknown SMS_PROVIDER %q", cfg.SMSProvider)
	}
}

// ---- console (development) --------------------------------------------------

type consoleEmail struct{ log *slog.Logger }

func (c consoleEmail) Send(_ context.Context, e Email) error {
	c.log.Info("EMAIL (console provider)", "to", e.To, "subject", e.Subject, "text", e.Text)
	return nil
}

type consoleSMS struct{ log *slog.Logger }

func (c consoleSMS) Send(_ context.Context, to, body string) error {
	c.log.Info("SMS (console provider)", "to", to, "body", body)
	return nil
}

type noSMS struct{}

func (noSMS) Send(context.Context, string, string) error {
	return fmt.Errorf("SMS is disabled (SMS_PROVIDER=none)")
}

// ---- SMTP -------------------------------------------------------------------

type smtpSender struct {
	from string
	opts []mail.Option
	host string
}

func newSMTP(cfg *config.Config) (EmailSender, error) {
	opts := []mail.Option{mail.WithPort(cfg.SMTPPort), mail.WithTimeout(20 * time.Second)}
	switch cfg.SMTPTLS {
	case "tls":
		opts = append(opts, mail.WithSSL())
	case "starttls":
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	}
	if cfg.SMTPUser != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(cfg.SMTPUser), mail.WithPassword(cfg.SMTPPass))
	}
	return &smtpSender{from: cfg.SMTPFrom, opts: opts, host: cfg.SMTPHost}, nil
}

func (s *smtpSender) Send(ctx context.Context, e Email) error {
	m := mail.NewMsg()
	if err := m.From(s.from); err != nil {
		return err
	}
	if err := m.To(e.To); err != nil {
		return err
	}
	m.Subject(e.Subject)
	m.SetBodyString(mail.TypeTextPlain, e.Text)
	if e.HTML != "" {
		m.AddAlternativeString(mail.TypeTextHTML, e.HTML)
	}
	c, err := mail.NewClient(s.host, s.opts...)
	if err != nil {
		return err
	}
	return c.DialAndSendWithContext(ctx, m)
}

// ---- Twilio -----------------------------------------------------------------

type twilio struct {
	sid, token, from string
	c                *http.Client
}

func (t twilio) Send(ctx context.Context, to, body string) error {
	form := url.Values{"To": {to}, "From": {t.from}, "Body": {body}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.twilio.com/2010-04-01/Accounts/"+t.sid+"/Messages.json", strings.NewReader(form.Encode()))
	req.SetBasicAuth(t.sid, t.token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return do(t.c, req)
}

// ---- generic HTTP gateway ---------------------------------------------------

type httpSMS struct {
	url, authHeader, bodyTpl string
	c                        *http.Client
}

func (h httpSMS) Send(ctx context.Context, to, body string) error {
	esc := func(s string) string { b, _ := json.Marshal(s); return string(b[1 : len(b)-1]) }
	payload := strings.NewReplacer("{{to}}", esc(to), "{{message}}", esc(body)).Replace(h.bodyTpl)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if k, v, ok := strings.Cut(h.authHeader, ":"); ok {
		req.Header.Set(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	return do(h.c, req)
}

func do(c *http.Client, req *http.Request) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("sms provider: %s: %s", resp.Status, b)
	}
	return nil
}

// ---- templates --------------------------------------------------------------

type tpl struct{ Subject, Text, HTML string }

// Templates are code so they are versioned, reviewed and tested. Add a key
// here and enqueue {"template": "<key>", "data": {...}}. Data always gets AppName.
var templates = map[string]tpl{
	"otp": {
		Subject: "{{.Code}} is your {{.AppName}} code",
		Text:    "Your {{.AppName}} verification code is {{.Code}}.\nIt expires in {{.Minutes}} minutes.\n\nIf you did not request this, you can ignore this email.",
		HTML:    `<p>Your {{.AppName}} verification code is</p><p style="font-size:28px;font-weight:700;letter-spacing:6px">{{.Code}}</p><p>It expires in {{.Minutes}} minutes. If you did not request this, you can ignore this email.</p>`,
	},
	"account_deleted": {
		Subject: "Your {{.AppName}} account is scheduled for deletion",
		Text:    "We received a request to delete your account. It will be permanently erased on {{.PurgeDate}}. Sign in before then to cancel.",
		HTML:    `<p>We received a request to delete your account. It will be permanently erased on <b>{{.PurgeDate}}</b>.</p><p>Sign in before then to cancel.</p>`,
	},
}

func Render(appName, name string, data map[string]any) (Email, error) {
	t, ok := templates[name]
	if !ok {
		return Email{}, fmt.Errorf("unknown email template %q", name)
	}
	if data == nil {
		data = map[string]any{}
	}
	data["AppName"] = appName
	var e Email
	var err error
	if e.Subject, err = execText(t.Subject, data); err != nil {
		return e, err
	}
	if e.Text, err = execText(t.Text, data); err != nil {
		return e, err
	}
	var buf bytes.Buffer
	h, err := htmltpl.New("h").Parse(layoutStart + t.HTML + layoutEnd)
	if err != nil {
		return e, err
	}
	if err := h.Execute(&buf, data); err != nil {
		return e, err
	}
	e.HTML = buf.String()
	return e, nil
}

func execText(s string, data any) (string, error) {
	t, err := template.New("t").Parse(s)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	err = t.Execute(&b, data)
	return b.String(), err
}

const layoutStart = `<!doctype html><html><body style="margin:0;background:#f5f5f7;font-family:-apple-system,Segoe UI,Roboto,sans-serif;color:#1d1d1f"><div style="max-width:520px;margin:24px auto;background:#fff;border-radius:12px;padding:28px">`
const layoutEnd = `<hr style="border:none;border-top:1px solid #eee;margin:24px 0"><p style="font-size:12px;color:#888">{{.AppName}}</p></div></body></html>`

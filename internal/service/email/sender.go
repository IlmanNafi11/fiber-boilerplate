package email

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"go.uber.org/zap"
)

// EmailSender defines the interface for sending transactional emails.
type EmailSender interface {
	SendVerificationEmail(ctx context.Context, to, token string) error
	SendPasswordResetEmail(ctx context.Context, to, token string) error
	SendPasswordChangedNotification(ctx context.Context, to string) error
}

// SMTPEmailSender implements EmailSender using net/smtp with STARTTLS.
type SMTPEmailSender struct {
	cfg    config.SMTPConfig
	logger *zap.Logger
}

// NewSMTPEmailSender creates a new SMTP-based email sender.
func NewSMTPEmailSender(cfg config.SMTPConfig, logger *zap.Logger) *SMTPEmailSender {
	return &SMTPEmailSender{cfg: cfg, logger: logger}
}

// SendVerificationEmail sends an email verification email with a single-use token.
func (s *SMTPEmailSender) SendVerificationEmail(_ context.Context, to, token string) error {
	subject := "Verify Your Email Address"
	body := fmt.Sprintf(
		"Please verify your email address by submitting the following token:\n\n%s\n\nThis token expires in 24 hours.\nIf you did not create an account, please ignore this email.",
		token,
	)
	return s.send(to, subject, body)
}

// SendPasswordResetEmail sends a password reset email with a single-use token.
func (s *SMTPEmailSender) SendPasswordResetEmail(_ context.Context, to, token string) error {
	subject := "Reset Your Password"
	body := fmt.Sprintf(
		"A password reset was requested for your account.\n\nYour reset token is:\n\n%s\n\nThis token expires in 15 minutes.\nIf you did not request a password reset, please ignore this email.",
		token,
	)
	return s.send(to, subject, body)
}

// SendPasswordChangedNotification sends a notification that password was changed.
func (s *SMTPEmailSender) SendPasswordChangedNotification(_ context.Context, to string) error {
	subject := "Your Password Has Been Changed"
	body := "Your password has been changed successfully. If you did not make this change, contact support immediately."
	return s.send(to, subject, body)
}

func (s *SMTPEmailSender) send(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	var auth smtp.Auth
	if s.cfg.User != "" {
		auth = smtp.PlainAuth("", s.cfg.User, s.cfg.Password, s.cfg.Host)
	}

	msg := strings.Join([]string{
		"From: " + s.cfg.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=\"utf-8\"",
		"",
		body,
	}, "\r\n")

	return smtp.SendMail(addr, auth, s.cfg.From, []string{to}, []byte(msg))
}

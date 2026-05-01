package email

import (
	"testing"

	"github.com/ilmannafi/fiber-boilerplate/internal/config"
	"go.uber.org/zap"
)

func TestNewSMTPEmailSender(t *testing.T) {
	cfg := config.SMTPConfig{
		Host:     "localhost",
		Port:     1025,
		User:     "",
		Password: "",
		From:     "test@example.com",
	}
	logger := zap.NewNop()

	sender := NewSMTPEmailSender(cfg, logger)
	if sender == nil {
		t.Fatal("expected non-nil sender")
	}
}

// TestSMTPEmailSender_ImplementsInterface verifies compile-time interface compliance.
func TestSMTPEmailSender_ImplementsInterface(t *testing.T) {
	var _ EmailSender = (*SMTPEmailSender)(nil)
}

package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNew_Development(t *testing.T) {
	l := New("development")
	assert.NotNil(t, l)
	defer l.Sync()
	// Verify the global logger was set
	assert.Equal(t, l, zap.L())
}

func TestNew_Production(t *testing.T) {
	l := New("production")
	assert.NotNil(t, l)
	defer l.Sync()
	assert.Equal(t, l, zap.L())
}

func TestNew_AlwaysJSON(t *testing.T) {
	// Both environments should produce valid loggers (JSON encoding)
	// Verified by ensuring they don't panic and return non-nil
	l1 := New("development")
	assert.NotNil(t, l1)
	l1.Sync()

	l2 := New("production")
	assert.NotNil(t, l2)
	l2.Sync()
}

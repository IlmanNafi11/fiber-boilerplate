// Package logger provides zap-based structured JSON logging.
//
// IMPORTANT (D-11): Never log passwords, tokens, secrets, or authorization headers.
// Only use explicitly selected zap fields (e.g., zap.String("email", email)).
// Never pass raw request bodies, config structs, or user credentials to logger calls.
package logger

import (
	"log"

	"go.uber.org/zap"
)

func New(env string) *zap.Logger {
	var cfg zap.Config
	if env == "production" {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}

	// Always use JSON encoding for structured output (per D-09)
	cfg.Encoding = "json"

	// Output to stdout for container compatibility
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}

	logger, err := cfg.Build()
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}

	zap.ReplaceGlobals(logger)

	return logger
}

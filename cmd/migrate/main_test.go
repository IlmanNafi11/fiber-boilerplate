package main

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateCLI_NoArgs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", ".")
	output, err := cmd.CombinedOutput()
	require.Error(t, err, "should exit non-zero with no args")

	out := string(output)
	assert.Contains(t, out, "usage: migrate <up|down|version|force> [flags]")
	assert.Contains(t, out, "up")
	assert.Contains(t, out, "down")
	assert.Contains(t, out, "version")
	assert.Contains(t, out, "force")
}

func TestMigrateCLI_ForceNoVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", ".", "force")
	output, err := cmd.CombinedOutput()
	require.Error(t, err, "should exit non-zero when force has no version argument")

	out := string(output)
	assert.Contains(t, out, "force requires a version argument")
}

func TestMigrateCLI_ForceInvalidVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", ".", "force", "abc")
	output, err := cmd.CombinedOutput()
	require.Error(t, err, "should exit non-zero when force version is not a number")

	out := string(output)
	assert.Contains(t, out, "invalid version")
}

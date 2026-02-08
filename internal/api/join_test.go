package api

import (
	"errors"
	"strings"
	"testing"

	"github.com/erraggy/oastools/joiner"
)

// =============================================================================
// parseSchemaStrategy Tests
// =============================================================================

func TestParseSchemaStrategy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected joiner.CollisionStrategy
	}{
		{name: "deduplicate", input: "deduplicate", expected: joiner.StrategyDeduplicateEquivalent},
		{name: "rename delegates", input: "rename", expected: joiner.StrategyRenameRight},
		{name: "first delegates", input: "first", expected: joiner.StrategyAcceptLeft},
		{name: "error delegates", input: "error", expected: joiner.StrategyFailOnCollision},
		{name: "empty defaults to rename", input: "", expected: joiner.StrategyRenameRight},
		{name: "invalid defaults to rename", input: "bogus", expected: joiner.StrategyRenameRight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSchemaStrategy(tt.input)
			if got != tt.expected {
				t.Errorf("parseSchemaStrategy(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// =============================================================================
// formatJoinError Tests
// =============================================================================

func TestFormatJoinError(t *testing.T) {
	t.Run("non-CollisionError returns generic message", func(t *testing.T) {
		err := errors.New("something broke")
		msg := formatJoinError(err)
		if !strings.Contains(msg, "join operation failed") {
			t.Errorf("expected generic prefix, got: %s", msg)
		}
		if !strings.Contains(msg, "something broke") {
			t.Errorf("expected original message, got: %s", msg)
		}
	})

	t.Run("CollisionError with FailOnCollision strategy", func(t *testing.T) {
		err := &joiner.CollisionError{
			Section:    "schemas",
			Key:        "User",
			FirstFile:  "api1.yaml",
			SecondFile: "api2.yaml",
			Strategy:   joiner.StrategyFailOnCollision,
		}
		msg := formatJoinError(err)
		if !strings.Contains(msg, "schemas") {
			t.Errorf("expected section in message, got: %s", msg)
		}
		if !strings.Contains(msg, "User") {
			t.Errorf("expected key in message, got: %s", msg)
		}
		if !strings.Contains(msg, "api1.yaml") {
			t.Errorf("expected first file in message, got: %s", msg)
		}
		if !strings.Contains(msg, "api2.yaml") {
			t.Errorf("expected second file in message, got: %s", msg)
		}
		if !strings.Contains(msg, "Current strategy: Error") {
			t.Errorf("expected strategy label 'Error', got: %s", msg)
		}
	})

	t.Run("CollisionError with AcceptLeft strategy", func(t *testing.T) {
		err := &joiner.CollisionError{
			Section:  "paths",
			Key:      "/users",
			Strategy: joiner.StrategyAcceptLeft,
		}
		msg := formatJoinError(err)
		if !strings.Contains(msg, "Current strategy: First") {
			t.Errorf("expected strategy label 'First', got: %s", msg)
		}
	})

	t.Run("CollisionError with RenameRight strategy", func(t *testing.T) {
		err := &joiner.CollisionError{
			Section:  "schemas",
			Key:      "Pet",
			Strategy: joiner.StrategyRenameRight,
		}
		msg := formatJoinError(err)
		if !strings.Contains(msg, "Current strategy: Rename") {
			t.Errorf("expected strategy label 'Rename', got: %s", msg)
		}
	})

	t.Run("CollisionError with Deduplicate strategy", func(t *testing.T) {
		err := &joiner.CollisionError{
			Section:  "schemas",
			Key:      "Order",
			Strategy: joiner.StrategyDeduplicateEquivalent,
		}
		msg := formatJoinError(err)
		if !strings.Contains(msg, "Current strategy: Deduplicate") {
			t.Errorf("expected strategy label 'Deduplicate', got: %s", msg)
		}
	})

	t.Run("CollisionError with unmapped strategy falls back to Error", func(t *testing.T) {
		err := &joiner.CollisionError{
			Section:  "schemas",
			Key:      "Item",
			Strategy: joiner.StrategyRenameLeft,
		}
		msg := formatJoinError(err)
		if !strings.Contains(msg, "Current strategy: Error") {
			t.Errorf("expected fallback strategy label 'Error', got: %s", msg)
		}
	})
}

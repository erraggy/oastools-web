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
		{name: "dedupOrRename", input: "dedupOrRename", expected: joiner.StrategyDeduplicateOrRename},
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

	t.Run("CollisionError with DeduplicateOrRename strategy", func(t *testing.T) {
		err := &joiner.CollisionError{
			Section:  "schemas",
			Key:      "Order",
			Strategy: joiner.StrategyDeduplicateOrRename,
		}
		msg := formatJoinError(err)
		if !strings.Contains(msg, "Current strategy: Deduplicate or Rename") {
			t.Errorf("expected strategy label 'Deduplicate or Rename', got: %s", msg)
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

// =============================================================================
// strategyComparesSchemas Tests
// =============================================================================

func TestStrategyComparesSchemas(t *testing.T) {
	tests := []struct {
		name     string
		strategy joiner.CollisionStrategy
		expected bool
	}{
		{name: "deduplicate equivalent compares", strategy: joiner.StrategyDeduplicateEquivalent, expected: true},
		{name: "deduplicate or rename compares", strategy: joiner.StrategyDeduplicateOrRename, expected: true},
		{name: "rename right does not", strategy: joiner.StrategyRenameRight, expected: false},
		{name: "accept left does not", strategy: joiner.StrategyAcceptLeft, expected: false},
		{name: "fail on collision does not", strategy: joiner.StrategyFailOnCollision, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strategyComparesSchemas(tt.strategy); got != tt.expected {
				t.Errorf("strategyComparesSchemas(%q) = %v, want %v", tt.strategy, got, tt.expected)
			}
		})
	}
}

// =============================================================================
// buildConsolidations Tests
// =============================================================================

func TestBuildConsolidations(t *testing.T) {
	t.Run("empty report yields nil", func(t *testing.T) {
		if got := buildConsolidations(nil); got != nil {
			t.Errorf("expected nil for an empty report, got: %#v", got)
		}
	})

	t.Run("maps survivor and folded names", func(t *testing.T) {
		got := buildConsolidations([]joiner.Consolidation{
			{
				Survivor:          "User",
				SurvivorGenerated: false,
				Folded: []joiner.FoldedName{
					{Name: "User_api2", Generated: true, Pointer: false},
					{Name: "Person", Generated: false, Pointer: true},
				},
			},
		})

		if len(got) != 1 {
			t.Fatalf("expected 1 consolidation, got %d", len(got))
		}
		c := got[0]
		if c.Survivor != "User" {
			t.Errorf("Survivor = %q, want %q", c.Survivor, "User")
		}
		if c.SurvivorGenerated {
			t.Error("SurvivorGenerated = true, want false")
		}
		if len(c.Folded) != 2 {
			t.Fatalf("expected 2 folded names, got %d", len(c.Folded))
		}
		if c.Folded[0].Name != "User_api2" || !c.Folded[0].Generated || c.Folded[0].Pointer {
			t.Errorf("unexpected first folded name: %#v", c.Folded[0])
		}
		if c.Folded[1].Name != "Person" || c.Folded[1].Generated || !c.Folded[1].Pointer {
			t.Errorf("unexpected second folded name: %#v", c.Folded[1])
		}
	})
}

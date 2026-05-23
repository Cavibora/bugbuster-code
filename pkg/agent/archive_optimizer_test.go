package agent

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestArchiveOptimizer_New(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)
	compactor := &mockCompactor{}
	optimizer := NewArchiveOptimizer(store, compactor)
	if optimizer == nil {
		t.Error("Expected non-nil optimizer")
	}
}

func TestArchiveOptimizer_Optimize_EmptyStore(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)
	compactor := &mockCompactor{}
	optimizer := NewArchiveOptimizer(store, compactor)

	err := optimizer.Optimize(context.Background())
	if err != nil {
		t.Errorf("Expected nil error for empty store, got: %v", err)
	}
}

func TestArchiveOptimizer_Optimize_WithBlocks(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)
	compactor := &mockCompactor{summarizeResult: "summarized"}
	optimizer := NewArchiveOptimizer(store, compactor)

	// Create some blocks
	for i := 0; i < 3; i++ {
		block := &ArchiveBlock{
			ID:          fmt.Sprintf("blk_opt_%d", i),
			SessionID:   "session1",
			Summary:     fmt.Sprintf("Block %d about testing", i),
			Topics:      []string{"testing", "go"},
			TokenCount:  100,
			CreatedAt:   time.Now().Add(-time.Duration(i) * time.Hour),
			SourcePhase: "compaction",
		}
		if err := store.SaveBlock(block); err != nil {
			t.Fatalf("SaveBlock error: %v", err)
		}
		if err := store.updateIndex(block); err != nil {
			t.Fatalf("updateIndex error: %v", err)
		}
	}

	err := optimizer.Optimize(context.Background())
	if err != nil {
		t.Errorf("Expected nil error, got: %v", err)
	}
}

func TestArchiveOptimizer_Optimize_Cancelled(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)
	compactor := &mockCompactor{}
	optimizer := NewArchiveOptimizer(store, compactor)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := optimizer.Optimize(ctx)
	// Should handle cancellation gracefully
	_ = err
}

func TestSearchContextTool_Interface(t *testing.T) {
	tool := NewSearchContextTool(nil)
	if tool.Name() != "search_context" {
		t.Errorf("Expected name 'search_context', got '%s'", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Expected non-empty description")
	}
	if tool.Parameters() == nil {
		t.Error("Expected non-nil parameters")
	}
}

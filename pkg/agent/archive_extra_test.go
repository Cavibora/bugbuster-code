package agent

import (
	"fmt"
	"testing"
	"time"
)

func TestArchiveStore_LoadBlockInSession(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)

	// Create a block in a session
	block := &ArchiveBlock{
		ID:          "blk_test_001",
		SessionID:   "session1",
		Summary:     "Test summary",
		Topics:      []string{"test"},
		TokenCount:  100,
		CreatedAt:   time.Now(),
		SourcePhase: "compaction",
	}
	if err := store.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock error: %v", err)
	}

	// Load it back
	loaded, err := store.LoadBlockInSession("blk_test_001", "session1")
	if err != nil {
		t.Fatalf("LoadBlockInSession error: %v", err)
	}
	if loaded.ID != "blk_test_001" {
		t.Errorf("Expected ID 'blk_test_001', got '%s'", loaded.ID)
	}
	if loaded.Summary != "Test summary" {
		t.Errorf("Expected summary 'Test summary', got '%s'", loaded.Summary)
	}

	// Load non-existent block
	_, err = store.LoadBlockInSession("nonexistent", "session1")
	if err == nil {
		t.Error("Expected error for non-existent block")
	}
}

func TestArchiveStore_SaveIndex(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)

	// Save index
	idx := &ArchiveIndex{
		Entries: []ArchiveIndexEntry{
			{
				ID:         "blk_001",
				Summary:    "Test block",
				Topics:     []string{"test"},
				TokenCount: 100,
				CreatedAt:  time.Now(),
				SessionID:  "session1",
			},
		},
	}

	if err := store.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex error: %v", err)
	}

	// Load it back
	loaded, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex error: %v", err)
	}
	if len(loaded.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(loaded.Entries))
	}
	if loaded.Entries[0].ID != "blk_001" {
		t.Errorf("Expected ID 'blk_001', got '%s'", loaded.Entries[0].ID)
	}
}

func TestArchiveStore_DeleteBlock(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)

	// Create a block
	block := &ArchiveBlock{
		ID:         "blk_delete_test",
		SessionID:  "session1",
		Summary:    "To be deleted",
		TokenCount: 50,
		CreatedAt:  time.Now(),
	}
	if err := store.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock error: %v", err)
	}

	// Verify it exists
	loaded, err := store.LoadBlockInSession("blk_delete_test", "session1")
	if err != nil {
		t.Fatalf("LoadBlockInSession error: %v", err)
	}
	if loaded.ID != "blk_delete_test" {
		t.Errorf("Expected ID 'blk_delete_test', got '%s'", loaded.ID)
	}

	// Delete it
	if err := store.DeleteBlock("blk_delete_test"); err != nil {
		t.Fatalf("DeleteBlock error: %v", err)
	}

	// Verify it's gone
	_, err = store.LoadBlock("blk_delete_test")
	if err == nil {
		t.Error("Expected error loading deleted block")
	}
}

func TestArchiveStore_DeleteBlockInSession(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)

	// Create a block
	block := &ArchiveBlock{
		ID:         "blk_delete_session",
		SessionID:  "session1",
		Summary:    "To be deleted",
		TokenCount: 50,
		CreatedAt:  time.Now(),
	}
	if err := store.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock error: %v", err)
	}

	// Delete it
	if err := store.DeleteBlockInSession("blk_delete_session", "session1"); err != nil {
		t.Fatalf("DeleteBlockInSession error: %v", err)
	}

	// Verify it's gone
	_, err := store.LoadBlockInSession("blk_delete_session", "session1")
	if err == nil {
		t.Error("Expected error loading deleted block")
	}
}

func TestArchiveStore_PruneBlocksForSession(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 2) // max 2 blocks

	// Create 4 blocks
	for i := 0; i < 4; i++ {
		block := &ArchiveBlock{
			ID:          fmt.Sprintf("blk_prune_%d", i),
			SessionID:   "session1",
			Summary:     fmt.Sprintf("Block %d", i),
			TokenCount:  50,
			CreatedAt:   time.Now().Add(time.Duration(i) * time.Minute),
			SourcePhase: "compaction",
		}
		if err := store.SaveBlock(block); err != nil {
			t.Fatalf("SaveBlock error: %v", err)
		}
		// Update index
		if err := store.updateIndex(block); err != nil {
			t.Fatalf("updateIndex error: %v", err)
		}
	}

	// Prune
	if err := store.PruneBlocksForSession("session1"); err != nil {
		t.Fatalf("PruneBlocksForSession error: %v", err)
	}

	// Verify only 2 blocks remain
	idx, err := store.LoadIndexForSession("session1")
	if err != nil {
		t.Fatalf("LoadIndexForSession error: %v", err)
	}
	if len(idx.Entries) > 2 {
		t.Errorf("Expected at most 2 entries after pruning, got %d", len(idx.Entries))
	}
}

func TestArchiveStore_PruneBlocks_NoPruningNeeded(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)

	// Create 1 block
	block := &ArchiveBlock{
		ID:         "blk_noprune",
		SessionID:  "session1",
		Summary:    "Should not be pruned",
		TokenCount: 50,
		CreatedAt:  time.Now(),
	}
	if err := store.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock error: %v", err)
	}
	if err := store.updateIndex(block); err != nil {
		t.Fatalf("updateIndex error: %v", err)
	}

	// Prune — should not remove anything
	if err := store.PruneBlocksForSession("session1"); err != nil {
		t.Fatalf("PruneBlocksForSession error: %v", err)
	}

	idx, err := store.LoadIndexForSession("session1")
	if err != nil {
		t.Fatalf("LoadIndexForSession error: %v", err)
	}
	if len(idx.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(idx.Entries))
	}
}

func TestArchiveStore_DeleteBlock_NonExistent(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)

	// Delete non-existent block — should not error
	if err := store.DeleteBlock("nonexistent"); err != nil {
		t.Errorf("Expected no error for non-existent block, got: %v", err)
	}
}

func TestArchiveStore_SaveIndexForSession(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)

	idx := &ArchiveIndex{
		Entries: []ArchiveIndexEntry{
			{
				ID:         "blk_001",
				Summary:    "Test",
				TokenCount: 100,
				CreatedAt:  time.Now(),
				SessionID:  "session1",
			},
		},
	}

	if err := store.SaveIndexForSession(idx, "session1"); err != nil {
		t.Fatalf("SaveIndexForSession error: %v", err)
	}

	loaded, err := store.LoadIndexForSession("session1")
	if err != nil {
		t.Fatalf("LoadIndexForSession error: %v", err)
	}
	if len(loaded.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(loaded.Entries))
	}
}

func TestArchiveStore_ListSessions(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)

	// Create sessions
	for _, session := range []string{"session1", "session2", "session3"} {
		block := &ArchiveBlock{
			ID:         "blk_" + session,
			SessionID:  session,
			Summary:    "Test",
			TokenCount: 50,
			CreatedAt:  time.Now(),
		}
		if err := store.SaveBlock(block); err != nil {
			t.Fatalf("SaveBlock error: %v", err)
		}
	}

	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions error: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("Expected 3 sessions, got %d", len(sessions))
	}
}

func TestArchiveStore_NewArchiveStore_DefaultMax(t *testing.T) {
	store := NewArchiveStore("/tmp/test", 0)
	if store.maxBlocks != 50 {
		t.Errorf("Expected default maxBlocks=50, got %d", store.maxBlocks)
	}
}

func TestGenerateBlockID(t *testing.T) {
	id := generateBlockID()
	if len(id) < 5 {
		t.Errorf("Expected block ID with length >= 5, got '%s'", id)
	}
	if id[:4] != "blk_" {
		t.Errorf("Expected block ID to start with 'blk_', got '%s'", id)
	}
}

func TestArchiveStore_PruneBlocks_NoOp(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 50)

	// PruneBlocks is now a no-op (returns nil)
	if err := store.PruneBlocks(); err != nil {
		t.Errorf("Expected nil from PruneBlocks, got: %v", err)
	}
}

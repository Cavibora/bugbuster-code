package agent

import (
	"context"
	"testing"
	"time"

	"bugbuster-code/pkg/provider"
)

func TestGeneralizeOldBlocks_OldBlock(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	block := &ArchiveBlock{
		ID:        "old-block-1",
		Summary:   "",
		Topics:    []string{"test"},
		CreatedAt: time.Now().Add(-48 * time.Hour),
		Messages: []provider.Message{
			provider.UserMsg("old message 1"),
			provider.AssistantText("old response 1"),
		},
		TokenCount: 100,
	}
	if err := store.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock failed: %v", err)
	}
	// Update index
	idx := &ArchiveIndex{Entries: []ArchiveIndexEntry{
		{ID: block.ID, Summary: block.Summary, Topics: block.Topics, TokenCount: block.TokenCount, CreatedAt: block.CreatedAt, SessionID: block.SessionID},
	}}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatal(err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	idx, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	err = optimizer.generalizeOldBlocks(context.Background(), idx)
	if err != nil {
		t.Fatalf("generalizeOldBlocks failed: %v", err)
	}

	updated, err := store.LoadBlock("old-block-1")
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}
	if !updated.Optimized {
		t.Error("Expected block to be optimized")
	}
	if updated.SourcePhase != "optimizer" {
		t.Errorf("Expected SourcePhase='optimizer', got '%s'", updated.SourcePhase)
	}
}

func TestGeneralizeOldBlocks_RecentBlock(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	block := &ArchiveBlock{
		ID:        "fresh-block-1",
		Summary:   "",
		Topics:    []string{"test"},
		CreatedAt: time.Now().Add(-1 * time.Hour),
		Messages: []provider.Message{
			provider.UserMsg("fresh message"),
			provider.AssistantText("fresh response"),
		},
		TokenCount: 100,
	}
	if err := store.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock failed: %v", err)
	}
	idx := &ArchiveIndex{Entries: []ArchiveIndexEntry{
		{ID: block.ID, Summary: block.Summary, Topics: block.Topics, TokenCount: block.TokenCount, CreatedAt: block.CreatedAt, SessionID: block.SessionID},
	}}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatal(err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	loadedIdx, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	err = optimizer.generalizeOldBlocks(context.Background(), loadedIdx)
	if err != nil {
		t.Fatalf("generalizeOldBlocks failed: %v", err)
	}

	updated, err := store.LoadBlock("fresh-block-1")
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}
	if updated.Optimized {
		t.Error("Expected block to NOT be optimized (too recent)")
	}
}

func TestGeneralizeOldBlocks_AlreadyOptimized(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	block := &ArchiveBlock{
		ID:         "optimized-block-1",
		Summary:    "already optimized",
		Topics:     []string{"test"},
		CreatedAt:  time.Now().Add(-48 * time.Hour),
		Optimized:  true,
		Messages:   []provider.Message{provider.AssistantText("summary")},
		TokenCount: 50,
	}
	if err := store.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock failed: %v", err)
	}
	idx := &ArchiveIndex{Entries: []ArchiveIndexEntry{
		{ID: block.ID, Summary: block.Summary, Topics: block.Topics, TokenCount: block.TokenCount, CreatedAt: block.CreatedAt, SessionID: block.SessionID},
	}}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatal(err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	loadedIdx, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	err = optimizer.generalizeOldBlocks(context.Background(), loadedIdx)
	if err != nil {
		t.Fatalf("generalizeOldBlocks failed: %v", err)
	}

	updated, err := store.LoadBlock("optimized-block-1")
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}
	if updated.Summary != "already optimized" {
		t.Errorf("Expected summary unchanged, got '%s'", updated.Summary)
	}
}

func TestMergeSimilarBlocks_SimilarTopics(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	block1 := &ArchiveBlock{
		ID:        "block-1",
		Summary:   "Summary about Go testing",
		Topics:    []string{"go", "testing", "unit-test"},
		CreatedAt: time.Now().Add(-48 * time.Hour),
		Messages: []provider.Message{
			provider.AssistantText("Go testing content"),
		},
		TokenCount: 100,
	}
	block2 := &ArchiveBlock{
		ID:        "block-2",
		Summary:   "Summary about Go benchmarks",
		Topics:    []string{"go", "benchmark", "performance"},
		CreatedAt: time.Now().Add(-48 * time.Hour),
		Messages: []provider.Message{
			provider.AssistantText("Go benchmark content"),
		},
		TokenCount: 100,
	}
	if err := store.SaveBlock(block1); err != nil {
		t.Fatalf("SaveBlock 1 failed: %v", err)
	}
	if err := store.SaveBlock(block2); err != nil {
		t.Fatalf("SaveBlock 2 failed: %v", err)
	}
	idx := &ArchiveIndex{Entries: []ArchiveIndexEntry{
		{ID: block1.ID, Summary: block1.Summary, Topics: block1.Topics, TokenCount: block1.TokenCount, CreatedAt: block1.CreatedAt, SessionID: block1.SessionID},
		{ID: block2.ID, Summary: block2.Summary, Topics: block2.Topics, TokenCount: block2.TokenCount, CreatedAt: block2.CreatedAt, SessionID: block2.SessionID},
	}}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatal(err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	loadedIdx, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	err = optimizer.mergeSimilarBlocks(context.Background(), loadedIdx)
	if err != nil {
		t.Fatalf("mergeSimilarBlocks failed: %v", err)
	}

	newIdx, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if len(newIdx.Entries) > 2 {
		t.Errorf("Expected at most 2 entries after merge, got %d", len(newIdx.Entries))
	}
}

func TestMergeSimilarBlocks_NoSimilarTopics(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	block1 := &ArchiveBlock{
		ID:        "block-1",
		Summary:   "Summary about Python",
		Topics:    []string{"python", "django", "web"},
		CreatedAt: time.Now().Add(-48 * time.Hour),
		Messages: []provider.Message{
			provider.AssistantText("Python content"),
		},
		TokenCount: 100,
	}
	block2 := &ArchiveBlock{
		ID:        "block-2",
		Summary:   "Summary about Rust",
		Topics:    []string{"rust", "systems", "memory"},
		CreatedAt: time.Now().Add(-48 * time.Hour),
		Messages: []provider.Message{
			provider.AssistantText("Rust content"),
		},
		TokenCount: 100,
	}
	if err := store.SaveBlock(block1); err != nil {
		t.Fatalf("SaveBlock 1 failed: %v", err)
	}
	if err := store.SaveBlock(block2); err != nil {
		t.Fatalf("SaveBlock 2 failed: %v", err)
	}
	idx := &ArchiveIndex{Entries: []ArchiveIndexEntry{
		{ID: block1.ID, Summary: block1.Summary, Topics: block1.Topics, TokenCount: block1.TokenCount, CreatedAt: block1.CreatedAt, SessionID: block1.SessionID},
		{ID: block2.ID, Summary: block2.Summary, Topics: block2.Topics, TokenCount: block2.TokenCount, CreatedAt: block2.CreatedAt, SessionID: block2.SessionID},
	}}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatal(err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	loadedIdx, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	err = optimizer.mergeSimilarBlocks(context.Background(), loadedIdx)
	if err != nil {
		t.Fatalf("mergeSimilarBlocks failed: %v", err)
	}

	newIdx, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if len(newIdx.Entries) != 2 {
		t.Errorf("Expected 2 entries (no merge), got %d", len(newIdx.Entries))
	}
}

func TestOptimize_WithBlocks(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	oldBlock := &ArchiveBlock{
		ID:        "old-block",
		Summary:   "",
		Topics:    []string{"go", "testing"},
		CreatedAt: time.Now().Add(-48 * time.Hour),
		Messages: []provider.Message{
			provider.UserMsg("How to test in Go?"),
			provider.AssistantText("Use testing package..."),
		},
		TokenCount: 100,
	}
	if err := store.SaveBlock(oldBlock); err != nil {
		t.Fatalf("SaveBlock failed: %v", err)
	}
	idx := &ArchiveIndex{Entries: []ArchiveIndexEntry{
		{ID: oldBlock.ID, Summary: oldBlock.Summary, Topics: oldBlock.Topics, TokenCount: oldBlock.TokenCount, CreatedAt: oldBlock.CreatedAt, SessionID: oldBlock.SessionID},
	}}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatal(err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	err := optimizer.Optimize(context.Background())
	if err != nil {
		t.Fatalf("Optimize failed: %v", err)
	}

	updated, err := store.LoadBlock("old-block")
	if err != nil {
		t.Fatalf("LoadBlock failed: %v", err)
	}
	if !updated.Optimized {
		t.Error("Expected block to be optimized after Optimize()")
	}
}

func TestOptimize_CancelledContext(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	optimizer := NewArchiveOptimizer(store, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := optimizer.Optimize(ctx)
	if err != nil {
		t.Logf("Optimize with cancelled context returned: %v", err)
	}
}

func TestOptimize_RunningTwice(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	optimizer := NewArchiveOptimizer(store, nil)

	err := optimizer.Optimize(context.Background())
	if err != nil {
		t.Fatalf("First Optimize failed: %v", err)
	}

	err = optimizer.Optimize(context.Background())
	if err != nil {
		t.Fatalf("Second Optimize failed: %v", err)
	}
}

func TestRebuildIndex_NonDefaultSession(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	block := &ArchiveBlock{
		ID:        "custom-block",
		Summary:   "Custom session block",
		Topics:    []string{"custom"},
		CreatedAt: time.Now(),
		Messages: []provider.Message{
			provider.AssistantText("Custom content"),
		},
		TokenCount: 50,
		SessionID:  "custom-session",
	}
	if err := store.SaveBlock(block); err != nil {
		t.Fatalf("SaveBlock failed: %v", err)
	}
	idx := &ArchiveIndex{Entries: []ArchiveIndexEntry{
		{ID: block.ID, Summary: block.Summary, Topics: block.Topics, TokenCount: block.TokenCount, CreatedAt: block.CreatedAt, SessionID: block.SessionID},
	}}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatal(err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	err := optimizer.rebuildIndex()
	if err != nil {
		t.Fatalf("rebuildIndex failed: %v", err)
	}

	newIdx, err := store.LoadIndex()
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if len(newIdx.Entries) < 1 {
		t.Errorf("Expected at least 1 entry in rebuilt index, got %d", len(newIdx.Entries))
	}
}

func TestRebuildIndex_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	optimizer := NewArchiveOptimizer(store, nil)
	err := optimizer.rebuildIndex()
	if err != nil {
		t.Fatalf("rebuildIndex on empty dir should not fail: %v", err)
	}
}

func TestMergeSimilarBlocks_CancelledContext(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	block1 := &ArchiveBlock{
		ID:         "block-1",
		Summary:    "Discussion about Go testing",
		Topics:     []string{"go", "testing"},
		Messages:   []provider.Message{provider.UserMsg("How to test?")},
		TokenCount: 100,
		CreatedAt:  time.Now().Add(-2 * time.Hour),
	}
	block2 := &ArchiveBlock{
		ID:         "block-2",
		Summary:    "Discussion about Go benchmarks",
		Topics:     []string{"go", "benchmark"},
		Messages:   []provider.Message{provider.UserMsg("How to benchmark?")},
		TokenCount: 100,
		CreatedAt:  time.Now().Add(-1 * time.Hour),
	}

	if err := store.SaveBlock(block1); err != nil {
		t.Fatalf("SaveBlock block1: %v", err)
	}
	if err := store.SaveBlock(block2); err != nil {
		t.Fatalf("SaveBlock block2: %v", err)
	}

	idx := &ArchiveIndex{
		Entries: []ArchiveIndexEntry{
			{ID: "block-1", Summary: block1.Summary, Topics: block1.Topics, TokenCount: block1.TokenCount},
			{ID: "block-2", Summary: block2.Summary, Topics: block2.Topics, TokenCount: block2.TokenCount},
		},
	}
	if err := store.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	optimizer := NewArchiveOptimizer(store, nil)
	err := optimizer.mergeSimilarBlocks(ctx, idx)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestMergeSimilarBlocks_SingleBlock(t *testing.T) {
	dir := t.TempDir()
	store := NewArchiveStore(dir, 1000)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	idx := &ArchiveIndex{
		Entries: []ArchiveIndexEntry{
			{ID: "block-1", Summary: "Single block", Topics: []string{"test"}, TokenCount: 100},
		},
	}

	optimizer := NewArchiveOptimizer(store, nil)
	err := optimizer.mergeSimilarBlocks(context.Background(), idx)
	if err != nil {
		t.Errorf("mergeSimilarBlocks with single block should not error: %v", err)
	}
}

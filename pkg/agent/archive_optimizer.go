package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/logger"
	"bugbuster-code/pkg/provider"
)

// ArchiveOptimizer is a new archive block optimizer.
// Runs during compaction for generalization, deduplication and truncation of archives.
type ArchiveOptimizer struct {
	store     *ArchiveStore
	compactor Compactor // LLM-compactor to generate summary (can be nil)
	mu        sync.Mutex
	running   bool
}

// NewArchiveOptimizer creates optimizer archives
func NewArchiveOptimizer(store *ArchiveStore, compactor Compactor) *ArchiveOptimizer {
	return &ArchiveOptimizer{
		store:     store,
		compactor: compactor,
	}
}

// Optimize starts one archive optimization loop.
// 1. Finds similar blocks (topic intersection ≥ 50%)
// 2. Merges them into one block with new summary
// 3. Generalizes old blocks (replaces Messages with summary)
// 4. Removes empty/redundant blocks
// 5. Updates index
func (o *ArchiveOptimizer) Optimize(ctx context.Context) error {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return nil
	}
	o.running = true
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.running = false
		o.mu.Unlock()
	}()

	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	idx, err := o.store.LoadIndex()
	if err != nil {
		return fmt.Errorf("archive optimizer: load index: %w", err)
	}

	if len(idx.Entries) == 0 {
		return nil
	}

	// Phase 1: Generalize old blocks (older than 24 hours, not optimized)
	if err := o.generalizeOldBlocks(ctx, idx); err != nil {
		logger.Error("archive optimizer: generalize", "err", err)
	}

	// Phase 2: Find and merge similar blocks
	if err := o.mergeSimilarBlocks(ctx, idx); err != nil {
		logger.Error("archive optimizer: merge", "err", err)
	}

	// Phase 3: Remove empty blocks
	if err := o.removeEmptyBlocks(idx); err != nil {
		logger.Error("archive optimizer: remove empty", "err", err)
	}

	// Update index
	_, err = o.store.LoadIndex()
	return err
}

// generalizeOldBlocks generalizes old blocks (older than 24 hours)
func (o *ArchiveOptimizer) generalizeOldBlocks(ctx context.Context, idx *ArchiveIndex) error {
	now := time.Now()
	changed := false

	for i := range idx.Entries {
		entry := &idx.Entries[i]

		// Skip already optimized and fresh blocks
		block, err := o.store.LoadBlock(entry.ID)
		if err != nil {
			continue
		}

		if block.Optimized {
			continue
		}

		// Generalize blocks older than 24 hours
		if now.Sub(block.CreatedAt) < 24*time.Hour {
			continue
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Generalize: replace full messages with summary
		summary := block.Summary
		if summary == "" {
			if o.compactor != nil {
				if ctxCompactor, ok := o.compactor.(interface {
					SummarizeWithCtx([]provider.Message, int, context.Context) string
				}); ok {
					summary = ctxCompactor.SummarizeWithCtx(block.Messages, 200, ctx)
				} else {
					summary = o.compactor.Summarize(block.Messages, 200)
				}
			} else {
				summary = SimpleSummarize(block.Messages, 200)
			}
		}

		// Replace messages with summary (assistant, not system — system not needed in archive)
		block.Messages = []provider.Message{
			{
				Role: "assistant",
				Content: []provider.ContentBlock{
					{Type: "text", Text: summary},
				},
			},
		}
		block.Summary = summary
		block.TokenCount = EstimateMessagesTokens(block.Messages)
		block.Optimized = true
		block.SourcePhase = "optimizer"

		if err := o.store.SaveBlock(block); err != nil {
			continue
		}

		// Update record in index
		entry.Summary = summary
		entry.TokenCount = block.TokenCount
		changed = true
	}

	if changed {
		return o.store.SaveIndex(idx)
	}
	return nil
}

// mergeSimilarBlocks finds and merges blocks with topic intersection ≥ 50%
func (o *ArchiveOptimizer) mergeSimilarBlocks(ctx context.Context, idx *ArchiveIndex) error {
	if len(idx.Entries) < 2 {
		return nil
	}

	// Find pairs with topic intersection ≥ 50%
	type mergePair struct {
		i, j int
	}
	var pairs []mergePair

	for i := 0; i < len(idx.Entries); i++ {
		for j := i + 1; j < len(idx.Entries); j++ {
			if topicSimilarity(idx.Entries[i].Topics, idx.Entries[j].Topics) >= 0.5 {
				pairs = append(pairs, mergePair{i, j})
			}
		}
	}

	if len(pairs) == 0 {
		return nil
	}

	// Merge pairs (take first pair, merge, delete second block)
	merged := make(map[int]bool)
	for _, pair := range pairs {
		if merged[pair.i] || merged[pair.j] {
			continue
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		blockI, err := o.store.LoadBlock(idx.Entries[pair.i].ID)
		if err != nil {
			continue
		}
		blockJ, err := o.store.LoadBlock(idx.Entries[pair.j].ID)
		if err != nil {
			continue
		}

		// Merge: take summary from both, merge topics
		mergedSummary := blockI.Summary
		if blockJ.Summary != "" {
			mergedSummary = mergedSummary + "\n\n" + i18n.T("archive.merged_with") + " " + blockJ.Summary
		}

		// Merge topics (unique)
		topicSet := make(map[string]bool)
		for _, t := range blockI.Topics {
			topicSet[t] = true
		}
		for _, t := range blockJ.Topics {
			topicSet[t] = true
		}
		mergedTopics := make([]string, 0, len(topicSet))
		for t := range topicSet {
			mergedTopics = append(mergedTopics, t)
		}

		// Update block I: merge messages, filter system
		mergedMessages := append(blockI.Messages, blockJ.Messages...)
		mergedMessages = filterMessagesForArchive(mergedMessages)
		blockI.Messages = mergedMessages
		blockI.Summary = mergedSummary
		blockI.Topics = mergedTopics
		blockI.TokenCount = EstimateMessagesTokens(mergedMessages)
		blockI.Optimized = true
		blockI.SourcePhase = "optimizer"

		if err := o.store.SaveBlock(blockI); err != nil {
			continue
		}

		// Delete block J
		_ = o.store.DeleteBlock(blockJ.ID)
		merged[pair.j] = true

		// Update record in index
		idx.Entries[pair.i].Summary = mergedSummary
		idx.Entries[pair.i].Topics = mergedTopics
		idx.Entries[pair.i].TokenCount = blockI.TokenCount
	}

	// Recreate index (removing merged entries)
	return o.rebuildIndex()
}

// removeEmptyBlocks deletes blocks with empty summary and zero TokenCount
func (o *ArchiveOptimizer) removeEmptyBlocks(idx *ArchiveIndex) error {
	removed := false
	for i := len(idx.Entries) - 1; i >= 0; i-- {
		entry := idx.Entries[i]
		if entry.Summary == "" && entry.TokenCount == 0 {
			_ = o.store.DeleteBlock(entry.ID)
			idx.Entries = append(idx.Entries[:i], idx.Entries[i+1:]...)
			removed = true
		}
	}
	if removed {
		return o.store.SaveIndex(idx)
	}
	return nil
}

// rebuildIndex recreates index from block files in all sessions
func (o *ArchiveOptimizer) rebuildIndex() error {
	// Walk all session subdirectories
	entries, err := os.ReadDir(o.store.dir)
	if err != nil {
		return err
	}

	var newEntries []ArchiveIndexEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()

		// Walk block files in session subdirectories
		sessionDir := filepath.Join(o.store.dir, sessionID)
		blockFiles, err := os.ReadDir(sessionDir)
		if err != nil {
			continue
		}

		for _, bf := range blockFiles {
			if !strings.HasSuffix(bf.Name(), ".json") || bf.Name() == "index.json" {
				continue
			}

			blockID := strings.TrimSuffix(bf.Name(), ".json")
			block, err := o.store.LoadBlockInSession(blockID, sessionID)
			if err != nil {
				continue
			}

			newEntries = append(newEntries, ArchiveIndexEntry{
				ID:         block.ID,
				Summary:    block.Summary,
				Topics:     block.Topics,
				TokenCount: block.TokenCount,
				CreatedAt:  block.CreatedAt,
				SessionID:  block.SessionID,
			})
		}
	}

	// Save index for each session separately
	// Group entries by sessionID
	sessionEntries := make(map[string][]ArchiveIndexEntry)
	for _, e := range newEntries {
		sid := e.SessionID
		if sid == "" {
			sid = "_default"
		}
		sessionEntries[sid] = append(sessionEntries[sid], e)
	}

	for sid, entries := range sessionEntries {
		idx := &ArchiveIndex{Entries: entries}
		if err := o.store.SaveIndexForSession(idx, sid); err != nil {
			return err
		}
	}

	return nil
}

// topicSimilarity computes intersection ratio between two topic sets
func topicSimilarity(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	setA := make(map[string]bool)
	for _, t := range a {
		setA[strings.ToLower(t)] = true
	}

	matches := 0
	for _, t := range b {
		if setA[strings.ToLower(t)] {
			matches++
		}
	}

	// Intersection ratio from smaller set
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	return float64(matches) / float64(minLen)
}

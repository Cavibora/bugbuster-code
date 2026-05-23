package agent

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/tools"
)

// SearchContextTool — tool for searching archived context
type SearchContextTool struct {
	Store     *ArchiveStore
	SessionID string // ID of current session for priority search
}

// NewSearchContextTool creates tool for searching archive
func NewSearchContextTool(store *ArchiveStore) *SearchContextTool {
	return &SearchContextTool{Store: store}
}

// Name returns tool name
func (t *SearchContextTool) Name() string {
	return "search_context"
}

// Description returns description tool
func (t *SearchContextTool) Description() string {
	return i18n.T("tools.search_context.description")
}

// Parameters returns JSON Schema parameters
func (t *SearchContextTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": i18n.T("tools.search_context.query_desc"),
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": i18n.T("tools.search_context.max_results_desc"),
			},
		},
		"required": []string{"query"},
	}
}

// Execute executes search in archive context
func (t *SearchContextTool) Execute(params map[string]string) tools.ToolResult {
	query := params["query"]
	if query == "" {
		return tools.ToolResult{Error: i18n.T("tools.search_context.empty_query")}
	}

	maxResults := 5
	if mr, ok := params["max_results"]; ok && mr != "" {
		var err error
		maxResults, err = parseInt(mr)
		if err != nil || maxResults <= 0 {
			maxResults = 5
		}
		if maxResults > 20 {
			maxResults = 20
		}
	}

	// Load index: first current session, then all others
	var allEntries []ArchiveIndexEntry

	// Determine current session
	sessionID := t.SessionID
	if sessionID == "" {
		sessionID = "_default"
	}

	// First search in current session
	sessionIdx, err := t.Store.LoadIndexForSession(sessionID)
	if err == nil && len(sessionIdx.Entries) > 0 {
		allEntries = append(allEntries, sessionIdx.Entries...)
	}

	// Then search in all other sessions
	otherSessions, err := t.Store.ListSessions()
	if err == nil {
		for _, sid := range otherSessions {
			if sid == sessionID {
				continue // Already loaded
			}
			sessionIdx, err := t.Store.LoadIndexForSession(sid)
			if err == nil && len(sessionIdx.Entries) > 0 {
				allEntries = append(allEntries, sessionIdx.Entries...)
			}
		}
	}

	if len(allEntries) == 0 {
		return tools.ToolResult{Output: i18n.T("tools.search_context.no_results")}
	}

	idx := &ArchiveIndex{Entries: allEntries}

	// Rank blocks by relevance
	scored := t.rankBlocks(idx, query)

	// Take top-N
	type result struct {
		entry ArchiveIndexEntry
		block *ArchiveBlock
	}
	results := make([]result, 0, maxResults)
	for i := 0; i < min(len(scored), maxResults); i++ {
		if scored[i].score == 0 {
			continue
		}
		block, err := t.Store.LoadBlock(scored[i].id)
		if err != nil {
			continue
		}
		results = append(results, result{entry: scored[i].entry, block: block})
	}

	if len(results) == 0 {
		return tools.ToolResult{Output: i18n.T("tools.search_context.no_results")}
	}

	// Format results with token budget
	tokenBudget := 2000
	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		blockText := FormatArchiveBlock(r.block, tokenBudget/len(results))
		sb.WriteString(blockText)
	}

	return tools.ToolResult{Output: sb.String()}
}

// scoredEntry — index record with relevance score
type scoredEntry struct {
	id    string
	entry ArchiveIndexEntry
	score float64
}

// rankBlocks ranks blocks by relevance to request
func (t *SearchContextTool) rankBlocks(idx *ArchiveIndex, query string) []scoredEntry {
	queryWords := tokenizeAndLower(query)
	if len(queryWords) == 0 {
		return nil
	}

	scored := make([]scoredEntry, 0, len(idx.Entries))
	for _, entry := range idx.Entries {
		score := 0.0

		// Topic matches (weight 3)
		for _, topic := range entry.Topics {
			topicLower := strings.ToLower(topic)
			for _, qw := range queryWords {
				if strings.Contains(topicLower, qw) {
					score += 3.0
				}
			}
		}

		// Summary matches (weight 1)
		summaryLower := strings.ToLower(entry.Summary)
		for _, qw := range queryWords {
			if strings.Contains(summaryLower, qw) {
				score += 1.0
			}
		}

		// Recency bonus (weight 0.5, max 5 points for freshness)
		hoursAgo := fmtTimeSince(entry.CreatedAt)
		if hoursAgo < 1 {
			score += 5.0
		} else if hoursAgo < 24 {
			score += 2.0
		} else if hoursAgo < 168 { // 7 days
			score += 1.0
		}

		scored = append(scored, scoredEntry{
			id:    entry.ID,
			entry: entry,
			score: score,
		})
	}

	// Sort by relevance descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	return scored
}

// tokenizeAndLower splits line into tokens and converts to lowercase
func tokenizeAndLower(s string) []string {
	s = strings.ToLower(s)
	words := strings.Fields(s)
	var result []string
	for _, w := range words {
		w = strings.Trim(w, ".,!?;:()[]{}\"'`")
		if len(w) > 1 {
			result = append(result, w)
		}
	}
	return result
}

// fmtTimeSince returns hours since time t
func fmtTimeSince(t time.Time) float64 {
	d := time.Since(t)
	return d.Hours()
}

// parseInt parses line into int
func parseInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid integer: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

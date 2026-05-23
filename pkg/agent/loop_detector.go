package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"bugbuster-code/pkg/i18n"
	"bugbuster-code/pkg/provider"
)

// LoopDetector detects agent loops.
// Tracks tool call patterns and model text responses.
// If the model repeats the same actions — interrupts execution.
type LoopDetector struct {
	mu sync.Mutex

	// History of iteration snapshots for loop detection.
	// Snapshot = hash of (tool name + parameters + result).
	snapshots []loopSnapshot

	// Maximum count of stored snapshots (sliding window).
	windowSize int

	// Threshold: if N identical consecutive snapshots — it's a loop.
	repeatThreshold int

	// Threshold: if N snapshots with same tool+params (ignoring result) — loop.
	// This catches the case when the model calls the same tool with the same parameters,
	// but receives an error and tries again.
	toolRepeatThreshold int

	// Threshold: if N consecutive text responses have similarity > textSimilarityThreshold — loop.
	// Catches "thinking loops" when the model rephrases the same thought.
	textSimilarityThreshold float64

	// How many recent text responses to check for similarity.
	textSimilarityWindow int

	// Threshold: if N consecutive thinking blocks have similarity > thinkingSimilarityThreshold — loop.
	// Catches "thinking loops" when the model thinks endlessly without taking action.
	thinkingSimilarityThreshold float64

	// How many recent thinking blocks to check for similarity.
	thinkingSimilarityWindow int
}

// loopSnapshot is a snapshot of one iteration (tool call + context)
type loopSnapshot struct {
	// Hash of full call: tool_name + params
	toolCallHash string

	// Tool name (for human-readable messages)
	toolName string

	// Tool parameters (for diagnostics)
	params map[string]string

	// Whether the call was successful
	ok bool

	// Hash of model text response (if not tool calls)
	textHash string

	// Set of significant words from text response (for similarity detection)
	textWords map[string]struct{}

	// Tool name + parameters for ping-pong grouping
	// (needed to distinguish read(a.go) → grep(TODO) → read(b.go) → grep(FIXME)
	//  from read(a.go) → grep(TODO) → read(a.go) → grep(TODO))
	toolAndParamsHash string

	// Hash of thinking block (for thinking loop detection)
	thinkingHash string

	// Set of significant words from thinking block (for similarity detection)
	thinkingWords map[string]struct{}
}

// NewLoopDetector creates detector with default parameters.
//   - windowSize: sliding window size (how many recent iterations to analyze)
//   - repeatThreshold: how many identical consecutive full calls = loop
//   - toolRepeatThreshold: how many calls of same tool with same parameters = loop
func NewLoopDetector() *LoopDetector {
	return &LoopDetector{
		windowSize:                  30,
		repeatThreshold:             6,
		toolRepeatThreshold:         8,
		textSimilarityThreshold:     0.65,
		textSimilarityWindow:        4,
		thinkingSimilarityThreshold: 0.65,
		thinkingSimilarityWindow:    3,
	}
}

// SetWindowSize sets sliding window size.
func (d *LoopDetector) SetWindowSize(n int) {
	if n > 0 {
		d.windowSize = n
	}
}

// SetRepeatThreshold sets the repetition threshold for loop detection.
func (d *LoopDetector) SetRepeatThreshold(n int) {
	if n > 0 {
		d.repeatThreshold = n
	}
}

// SetToolRepeatThreshold sets the repetition threshold for single tool calls.
func (d *LoopDetector) SetToolRepeatThreshold(n int) {
	if n > 0 {
		d.toolRepeatThreshold = n
	}
}

// SetTextSimilarityThreshold sets the text response similarity threshold (0.0-1.0).
func (d *LoopDetector) SetTextSimilarityThreshold(t float64) {
	if t > 0 && t <= 1.0 {
		d.textSimilarityThreshold = t
	}
}

// SetTextSimilarityWindow sets count of text responses for similarity check.
func (d *LoopDetector) SetTextSimilarityWindow(n int) {
	if n > 0 {
		d.textSimilarityWindow = n
	}
}

// RecordToolCall records tool call and checks for loop.
// Returns (isLoop, message). If isLoop=true — must interrupt agent loop.
func (d *LoopDetector) RecordToolCall(toolName string, params map[string]string, ok bool) (isLoop bool, message string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	callHash := hashToolCall(toolName, params)

	snap := loopSnapshot{
		toolCallHash:      callHash,
		toolName:          toolName,
		params:            params,
		ok:                ok,
		toolAndParamsHash: callHash, // same as toolCallHash (tool + params)
	}

	d.snapshots = append(d.snapshots, snap)
	d.trim()

	return d.detect()
}

// RecordTextResponse records model text response (without tool calls) and checks for loop.
func (d *LoopDetector) RecordTextResponse(text string) (isLoop bool, message string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	snap := loopSnapshot{
		textHash:  hashString(text),
		textWords: extractWords(text),
	}

	d.snapshots = append(d.snapshots, snap)
	d.trim()

	return d.detect()
}

// RecordThinking records model thinking block and checks for thinking loop.
// Returns (isLoop, message). If isLoop=true — model is stuck in a thinking loop.
func (d *LoopDetector) RecordThinking(thinking string) (isLoop bool, message string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	snap := loopSnapshot{
		thinkingHash:  hashString(thinking),
		thinkingWords: extractWords(thinking),
	}

	d.snapshots = append(d.snapshots, snap)
	d.trim()

	return d.detect()
}

// Reset resets detector history (e.g., on /reset).
func (d *LoopDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.snapshots = d.snapshots[:0]
}

// Stats returns detector statistics for debugging.
func (d *LoopDetector) Stats() (totalSnapshots int, topPattern string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	totalSnapshots = len(d.snapshots)

	// Find the most frequent pattern
	counts := make(map[string]int)
	for _, s := range d.snapshots {
		key := s.toolCallHash
		if key == "" {
			key = "text:" + s.textHash
		}
		counts[key]++
	}

	maxCount := 0
	for key, count := range counts {
		if count > maxCount {
			maxCount = count
			topPattern = key
		}
	}

	return
}

// trim trims history to windowSize (sliding window).
func (d *LoopDetector) trim() {
	if len(d.snapshots) > d.windowSize {
		d.snapshots = d.snapshots[len(d.snapshots)-d.windowSize:]
	}
}

// detect checks history for loops.
// Called under mutex.
func (d *LoopDetector) detect() (isLoop bool, message string) {
	n := len(d.snapshots)
	// At least 2 snapshots for any detection
	if n < 2 {
		return false, ""
	}

	// ────────────────────────────────────────────────────
	// Heuristic 1: consecutive identical calls
	// (tool + params + result match)
	// ────────────────────────────────────────────────────
	last := d.snapshots[n-1]
	if last.toolCallHash != "" {
		consecutive := 1
		for i := n - 2; i >= 0; i-- {
			if d.snapshots[i].toolCallHash == last.toolCallHash {
				consecutive++
			} else {
				break
			}
		}
		if consecutive >= d.repeatThreshold {
			return true, formatLoopMessage(last.toolName, last.params, consecutive, "identical_calls")
		}
	}

	// ────────────────────────────────────────────────────
	// Heuristic 2: same tool with same
	// parameters, but different results (model tries
	// again and again, possibly with errors)
	// ────────────────────────────────────────────────────
	if last.toolCallHash != "" {
		sameCallCount := 0
		for _, s := range d.snapshots {
			if s.toolCallHash == last.toolCallHash {
				sameCallCount++
			}
		}
		if sameCallCount >= d.toolRepeatThreshold {
			return true, formatLoopMessage(last.toolName, last.params, sameCallCount, "repeated_tool")
		}
	}

	// ────────────────────────────────────────────────────
	// Heuristic 3: model repeats the same text
	// response without tool calls
	// ────────────────────────────────────────────────────
	if last.textHash != "" {
		consecutive := 1
		for i := n - 2; i >= 0; i-- {
			if d.snapshots[i].textHash == last.textHash && d.snapshots[i].textHash != "" {
				consecutive++
			} else {
				break
			}
		}
		if consecutive >= d.repeatThreshold {
			return true, formatLoopMessageText(consecutive)
		}
	}

	// ────────────────────────────────────────────────────
	// Heuristic 4: "ping-pong" between two calls
	// with same parameters
	// (A(params1) → B(params2) → A(params1) → B(params2) → ...)
	// Differs from regular repeats in that model alternates
	// two DIFFERENT calls, but with same parameters each time.
	//
	// In this case read(a.go) → grep(TODO) → read(b.go) → grep(FIXME)
	// is NOT ping-pong — model works with different files.
	// ────────────────────────────────────────────────────
	if n >= 6 && d.snapshots[n-1].toolAndParamsHash != "" && d.snapshots[n-2].toolAndParamsHash != "" {
		hashA := d.snapshots[n-1].toolAndParamsHash
		hashB := d.snapshots[n-2].toolAndParamsHash
		if hashA == hashB {
			// This is not ping-pong, these are repeats — caught by heuristic 1
		} else {
			pingPong := true
			for i := n - 3; i >= n-6 && i >= 0; i-- {
				expected := hashA
				if (n-1-i)%2 == 1 {
					expected = hashB
				}
				if d.snapshots[i].toolAndParamsHash != expected {
					pingPong = false
					break
				}
			}
			if pingPong {
				toolA := d.snapshots[n-1].toolName
				toolB := d.snapshots[n-2].toolName
				return true, formatLoopMessagePingPong(toolA, toolB)
			}
		}
	}

	// ────────────────────────────────────────────────────
	// Heuristic 5: semantic similarity of text responses
	// Model may rephrase the same thought,
	// without making tool calls — "thinking loop".
	// If last N text responses have high
	// word intersection (Jaccard similarity > threshold) — loop.
	// ────────────────────────────────────────────────────
	if last.textWords != nil && d.textSimilarityWindow > 0 {
		// Collect recent text snapshots
		var textSnaps []loopSnapshot
		for i := n - 1; i >= 0 && len(textSnaps) < d.textSimilarityWindow; i-- {
			if d.snapshots[i].textWords != nil {
				textSnaps = append(textSnaps, d.snapshots[i])
			}
		}
		if len(textSnaps) >= d.textSimilarityWindow {
			// Check pairwise similarity of all text responses in window
			allSimilar := true
			reference := textSnaps[0].textWords
			for i := 1; i < len(textSnaps); i++ {
				sim := jaccardSimilarity(reference, textSnaps[i].textWords)
				if sim < d.textSimilarityThreshold {
					allSimilar = false
					break
				}
			}
			if allSimilar {
				return true, formatLoopMessageTextSimilar(len(textSnaps))
			}
		}
	}

	// ────────────────────────────────────────────────────
	// Heuristic 6: thinking loop
	// Model repeats the same thinking block,
	// without taking action. This is typical for GLM-5.1
	// via z.ai — model "chews gum", thinking
	// about opening a file, but not opening it.
	// If last N thinking blocks have high
	// word intersection — this is a thinking loop.
	// ────────────────────────────────────────────────────
	if last.thinkingWords != nil && d.thinkingSimilarityWindow > 0 {
		var thinkingSnaps []loopSnapshot
		for i := n - 1; i >= 0 && len(thinkingSnaps) < d.thinkingSimilarityWindow; i-- {
			if d.snapshots[i].thinkingWords != nil {
				thinkingSnaps = append(thinkingSnaps, d.snapshots[i])
			}
		}
		if len(thinkingSnaps) >= d.thinkingSimilarityWindow {
			allSimilar := true
			reference := thinkingSnaps[0].thinkingWords
			for i := 1; i < len(thinkingSnaps); i++ {
				sim := jaccardSimilarity(reference, thinkingSnaps[i].thinkingWords)
				if sim < d.thinkingSimilarityThreshold {
					allSimilar = false
					break
				}
			}
			if allSimilar {
				return true, formatLoopMessageThinking(len(thinkingSnaps))
			}
		}
	}

	return false, ""
}

// ANSI escape codes for coloring loop messages.
// Duplicated from cmd/bugbuster/ui.go to avoid pkg → cmd dependency.
const (
	loopReset   = "\033[0m"
	loopBold    = "\033[1m"
	loopDim     = "\033[2m"
	loopRed     = "\033[31m"
	loopYellow  = "\033[33m"
	loopCyan    = "\033[36m"
	loopMagenta = "\033[35m"
)

// formatLoopMessage formats a colored human-readable loop message.
func formatLoopMessage(toolName string, params map[string]string, count int, pattern string) string {
	var sb strings.Builder

	sb.WriteString(loopBold + loopYellow + "🔄 " + i18n.T("loop_detector.detected") + loopReset + "\n")

	// Tool name — bright
	sb.WriteString("  " + loopBold + loopCyan + toolName + loopReset)

	// Repeat count — warning
	sb.WriteString(loopYellow + " × " + loopBold)
	sb.WriteString(fmt.Sprintf("%d", count))
	sb.WriteString(loopReset + "\n")

	// Show parameters (if any and not too long)
	if params != nil {
		sb.WriteString("  " + loopDim)
		for k, v := range params {
			display := v
			if len(display) > 60 {
				display = display[:57] + "..."
			}
			sb.WriteString(k + ": " + loopCyan + display + loopDim + "  ")
		}
		sb.WriteString(loopReset + "\n")
	}

	switch pattern {
	case "identical_calls":
		sb.WriteString("  " + loopDim + i18n.T("loop_detector.hint_identical") + loopReset)
	case "repeated_tool":
		sb.WriteString("  " + loopDim + i18n.T("loop_detector.hint_repeated") + loopReset)
	}

	return sb.String()
}

func formatLoopMessageText(count int) string {
	return loopBold + loopYellow + "🔄 " +
		i18n.T("loop_detector.text_repeated", count) + loopReset + "\n" +
		"  " + loopDim + i18n.T("loop_detector.hint_text") + loopReset
}

func formatLoopMessagePingPong(toolA, toolB string) string {
	msg := i18n.T("loop_detector.ping_pong",
		loopBold+loopCyan+toolA+loopReset,
		loopBold+loopCyan+toolB+loopReset,
	)
	return loopBold + loopYellow + "🔄 " + msg + loopReset + "\n" +
		"  " + loopDim + i18n.T("loop_detector.hint_ping_pong") + loopReset
}

// hashToolCall creates hash for tool call (name + parameters).
func hashToolCall(toolName string, params map[string]string) string {
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write([]byte{0}) // separator

	// Sort keys for deterministic hash
	paramsJSON, _ := json.Marshal(params)
	h.Write(paramsJSON)

	return fmt.Sprintf("%x", h.Sum(nil)[:16])
}

// hashString creates hash of lines.
func hashString(s string) string {
	if s == "" {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil)[:16])
}

// extractWords extracts set of significant words from text for similarity comparison.
// Removes stop words, punctuation, converts to lowercase.
func extractWords(text string) map[string]struct{} {
	words := make(map[string]struct{})
	// Stop words that don't carry semantic meaning (multilingual: en, ru, de, es, fr, pt, ja, zh)
	stopWords := map[string]struct{}{
		// English
		"the": {}, "a": {}, "an": {}, "is": {}, "are": {}, "was": {}, "were": {},
		"be": {}, "been": {}, "being": {}, "have": {}, "has": {}, "had": {},
		"do": {}, "does": {}, "did": {}, "will": {}, "would": {}, "could": {},
		"should": {}, "may": {}, "might": {}, "shall": {}, "can": {}, "need": {},
		"to": {}, "of": {}, "in": {}, "for": {}, "on": {}, "with": {}, "at": {},
		"by": {}, "from": {}, "as": {}, "into": {}, "through": {}, "during": {},
		"before": {}, "after": {}, "above": {}, "below": {}, "between": {},
		"and": {}, "but": {}, "or": {}, "nor": {}, "not": {}, "so": {}, "yet": {},
		"it": {}, "its": {}, "this": {}, "that": {}, "these": {}, "those": {},
		"i": {}, "me": {}, "my": {}, "we": {}, "our": {}, "you": {}, "your": {},
		"he": {}, "she": {}, "they": {}, "them": {}, "their": {}, "what": {},
		"which": {}, "who": {}, "whom": {}, "where": {}, "when": {}, "how": {},
		"all": {}, "each": {}, "every": {}, "both": {}, "few": {}, "more": {},
		"most": {}, "other": {}, "some": {}, "such": {}, "no": {}, "only": {},
		"same": {}, "than": {}, "too": {}, "very": {}, "just": {}, "also": {},
		// Russian
		"и": {}, "в": {}, "на": {}, "с": {}, "по": {}, "к": {}, "у": {}, "о": {},
		"а": {}, "но": {}, "не": {}, "как": {}, "то": {}, "это": {}, "для": {},
		"от": {}, "из": {}, "за": {}, "же": {}, "бы": {}, "ли": {}, "да": {},
		"этот": {}, "эта": {}, "этом": {}, "который": {}, "которая": {}, "которое": {},
		"которые": {}, "чтобы": {}, "потому": {}, "может": {}, "можно": {},
		"надо": {}, "будет": {}, "были": {},
		// German
		"der": {}, "die": {}, "das": {}, "ein": {}, "eine": {}, "ist": {}, "sind": {},
		"war": {}, "waren": {}, "sein": {}, "gewesen": {}, "haben": {}, "hat": {},
		"hatte": {}, "werden": {}, "wurde": {}, "wurden": {}, "können": {}, "kann": {},
		"könnte": {}, "sollte": {}, "muss": {}, "dürfen": {}, "dürfte": {}, "nicht": {},
		"auch": {}, "aber": {}, "oder": {}, "und": {}, "als": {}, "wie": {}, "von": {},
		"zu": {}, "auf": {}, "mit": {}, "für": {}, "bei": {},
		"nach": {}, "über": {}, "vor": {}, "zwischen": {}, "durch": {}, "um": {},
		"aus": {}, "noch": {}, "schon": {}, "nur": {}, "sehr": {}, "ja": {},
		"nein": {}, "dieser": {}, "diese": {}, "dieses": {}, "jener": {}, "jede": {},
		"jeder": {}, "jedem": {}, "allen": {}, "man": {}, "einem": {}, "einer": {},
		"eines": {}, "welche": {}, "welcher": {}, "welches": {}, "wo": {},
		"wann": {}, "wer": {}, "warum": {}, "wird": {},
		// Spanish
		"el": {}, "la": {}, "los": {}, "las": {}, "un": {}, "una": {}, "unos": {},
		"unas": {}, "son": {}, "era": {}, "eran": {}, "ser": {}, "estar": {},
		"está": {}, "están": {}, "han": {}, "había": {}, "puede": {},
		"pueden": {}, "podría": {}, "debería": {}, "también": {}, "pero": {},
		"como": {}, "más": {}, "muy": {}, "ya": {}, "este": {},
		"esta": {}, "esto": {}, "estos": {}, "estas": {}, "aquel": {}, "aquella": {},
		"aquello": {}, "quien": {}, "cual": {}, "cuando": {}, "donde": {},
		"por": {}, "para": {}, "con": {}, "sin": {}, "sobre": {}, "entre": {},
		"hasta": {}, "desde": {}, "todo": {}, "todos": {}, "cada": {}, "otro": {},
		"otra": {}, "otros": {}, "otras": {}, "sí": {}, "bien": {}, "poco": {},
		// French
		"les": {}, "des": {}, "du": {}, "était": {}, "étaient": {}, "être": {},
		"avoir": {}, "ont": {}, "avait": {}, "peut": {}, "peuvent": {},
		"pourrait": {}, "devrait": {}, "pas": {}, "aussi": {}, "ou": {},
		"très": {}, "déjà": {}, "ce": {}, "cette": {}, "ces": {}, "celui": {},
		"celle": {}, "ceux": {}, "qui": {}, "quoi": {}, "quand": {}, "où": {},
		"par": {}, "sans": {}, "dans": {}, "tout": {}, "tous": {}, "chaque": {},
		"autre": {}, "autres": {}, "oui": {}, "non": {}, "peu": {},
		// Portuguese
		"os": {}, "umas": {}, "uns": {}, "uma": {}, "são": {}, "eram": {},
		"estão": {}, "tem": {}, "têm": {}, "pode": {},
		"podem": {}, "poderia": {}, "deveria": {}, "não": {}, "também": {},
		"muito": {}, "já": {}, "isto": {}, "estes": {}, "aquele": {},
		"aquela": {}, "aquilo": {}, "quem": {}, "qual": {}, "quando": {}, "onde": {},
		"sem": {}, "até": {}, "outro": {},
		"outros": {}, "sim": {}, "bem": {}, "pouco": {},
		// Japanese
		"の": {}, "に": {}, "は": {}, "を": {}, "が": {}, "で": {}, "と": {},
		"も": {}, "から": {}, "まで": {}, "より": {}, "へ": {}, "や": {}, "など": {},
		"だ": {}, "である": {}, "です": {}, "ます": {}, "れる": {}, "られる": {},
		"しない": {}, "ない": {}, "ある": {}, "いる": {}, "する": {}, "なる": {},
		"この": {}, "その": {}, "あの": {}, "どの": {}, "これ": {}, "それ": {},
		"あれ": {}, "何": {}, "誰": {}, "どこ": {}, "いつ": {}, "なぜ": {},
		"どう": {}, "とても": {}, "もう": {}, "まだ": {}, "また": {}, "ただ": {},
		"しかし": {}, "そして": {}, "だから": {}, "または": {},
		// Chinese
		"了": {}, "在": {}, "我": {}, "有": {}, "就": {}, "人": {}, "都": {},
		"一个": {}, "上": {}, "很": {}, "到": {}, "说": {}, "要": {}, "去": {},
		"你": {}, "会": {}, "着": {}, "没有": {}, "看": {}, "好": {}, "自己": {},
		"这": {}, "他": {}, "她": {}, "它": {}, "们": {}, "那": {}, "些": {},
		"什么": {}, "哪": {}, "谁": {}, "怎么": {}, "为什么": {}, "哪里": {},
		"可以": {}, "能": {}, "应该": {}, "但是": {}, "因为": {},
		"所以": {}, "如果": {}, "虽然": {}, "还是": {}, "或者": {}, "而且": {},
	}

	var buf strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			buf.WriteRune(unicode.ToLower(r))
		} else if buf.Len() > 0 {
			word := buf.String()
			buf.Reset()
			if len(word) >= 3 { // Ignore too short words
				if _, isStop := stopWords[word]; !isStop {
					words[word] = struct{}{}
				}
			}
		}
	}
	// Last word
	if buf.Len() > 0 {
		word := buf.String()
		if len(word) >= 3 {
			if _, isStop := stopWords[word]; !isStop {
				words[word] = struct{}{}
			}
		}
	}
	return words
}

// jaccardSimilarity computes Jaccard coefficient between two word sets.
// Returns value from 0.0 (no common words) to 1.0 (identical sets).
func jaccardSimilarity(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	intersection := 0
	for k := range a {
		if _, ok := b[k]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// formatLoopMessageTextSimilar formats loop message by text similarity.
func formatLoopMessageTextSimilar(count int) string {
	return loopBold + loopYellow + "🔄 " +
		i18n.T("loop_detector.text_similar", count) + loopReset + "\n" +
		"  " + loopDim + i18n.T("loop_detector.hint_text_similar") + loopReset
}

// formatLoopMessageThinking formats thinking loop message.
func formatLoopMessageThinking(count int) string {
	return loopBold + loopYellow + "🔄 " +
		i18n.T("loop_detector.thinking_loop", count) + loopReset + "\n" +
		"  " + loopDim + i18n.T("loop_detector.hint_thinking_loop") + loopReset
}

// extractToolCallsInfo extracts tool call information from model response
// for passing to LoopDetector.
func extractToolCallsInfo(msg provider.Message) []struct {
	Name   string
	Params map[string]string
} {
	calls := msg.GetToolCalls()
	if len(calls) == 0 {
		return nil
	}

	var result []struct {
		Name   string
		Params map[string]string
	}

	for _, tc := range calls {
		params := convertInputToParams(tc.Input)
		result = append(result, struct {
			Name   string
			Params map[string]string
		}{
			Name:   tc.ToolName,
			Params: params,
		})
	}

	return result
}

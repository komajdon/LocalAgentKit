package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"agent-gui/domain"
	"agent-gui/internal/config"
	"agent-gui/internal/history"
	"agent-gui/internal/permission"
	"agent-gui/internal/recorder"
	"agent-gui/internal/store"
	"agent-gui/internal/whisper"
	"agent-gui/tools"
)

// displayItem mirrors the frontend Item type for serialising display history.
type displayItem struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// App is the Wails-bound facade. All public methods are exposed to the frontend.
type App struct {
	ctx context.Context

	startupErr string // set once during startup

	// convMu serialises read-modify-write of stored conversations. It is
	// taken before mu.
	convMu sync.Mutex

	mu            sync.Mutex
	cfg           config.Config
	activeID      string // conversation the UI is showing
	run           *run   // the single active run, or nil
	runSeq        uint64
	restoreStaged bool

	permissions *permission.Manager // set once during startup
	mcp         *tools.MCPPool
	pending     *pendingPrompt // guarded by mu
	promptSeq   uint64         // guarded by mu

	// globalTokens is the account-wide cumulative token count (prompt+completion
	// across all conversations) backing the shared usage budget. Guarded by mu.
	globalTokens int64

	recMu  sync.Mutex
	recSes *recorder.Session

	// Seams replaced by tests.
	emit        func(name string, data any)
	logf        func(format string, args ...any)
	providerFor func(config.Config) domain.LLMProvider
}

func NewApp() *App {
	// Without a store this cannot fail; startup reloads persisted grants.
	m, _ := permission.NewManager(nil)
	a := &App{permissions: m, providerFor: newProvider, mcp: tools.NewMCPPool()}
	a.emit = func(name string, data any) { runtime.EventsEmit(a.ctx, name, data) }
	a.logf = func(format string, args ...any) { runtime.LogErrorf(a.ctx, format, args...) }
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := store.Init(store.DataDir()); err != nil {
		// Surface the error to the user immediately rather than failing silently.
		// The event can fire before the UI listens; GetStartupError covers that.
		a.startupErr = "Failed to open data store: " + err.Error()
		runtime.EventsEmit(ctx, "app:fatal", a.startupErr)
	}
	if a.startupErr == "" {
		if err := history.Migrate(filepath.Join(store.DataDir(), "backups")); err != nil {
			// Nothing may be written to data this version cannot handle.
			a.startupErr = "Failed to upgrade the data store: " + err.Error()
			_ = store.Close()
			runtime.EventsEmit(ctx, "app:fatal", a.startupErr)
		}
	}
	a.cfg = config.Load()
	m, err := permission.NewManager(grantStore{})
	if err != nil {
		runtime.LogErrorf(ctx, "saved approvals could not be read; you will be asked again: %v", err)
	}
	a.permissions = m
	p, c := history.TotalUsage()
	a.globalTokens = p + c
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	restoring := a.restoreStaged
	active := a.run
	a.mu.Unlock()
	// Let a run stop and save before the store closes.
	if active != nil {
		a.StopAgent()
		select {
		case <-active.done:
		case <-time.After(shutdownWait):
		}
	}
	// After a restore was staged, keep the archives from before it intact.
	if !restoring && a.startupErr == "" {
		if err := writeAutoBackup(filepath.Join(store.DataDir(), "backups"), a.config().BackupRetention); err != nil {
			runtime.LogErrorf(ctx, "automatic backup failed: %v", err)
		}
	}
	a.mcp.Close()
	_ = store.Close()
}

// GetStartupError reports why the app could not start properly, or "".
func (a *App) GetStartupError() string { return a.startupErr }

// ── Branching ───────────────────────────────────────────────────────────────────

// BranchConversation creates a copy of the conversation up to the item at
// displayItemIndex. Branching from a user message sends that message again in
// the new branch, so it needs the agent to be idle.
func (a *App) BranchConversation(id string, displayItemIndex int) (*history.Conversation, error) {
	if a.GetRunState().RunID != "" {
		return nil, errors.New("stop the agent before branching")
	}
	src, err := history.Load(id)
	if err != nil {
		return nil, err
	}

	// Determine the cut point in the message slice.
	// Walk display items and saved messages in parallel so we know how many
	// saved messages to keep.
	items := parseDisplayItems(src.DisplayItems)
	if displayItemIndex < 0 || displayItemIndex >= len(items) {
		return nil, fmt.Errorf("invalid item index %d (max %d)", displayItemIndex, len(items)-1)
	}

	// Count how many visible user/assistant messages we need up to the branching
	// item. Branching from a user message copies the prior context and re-sends
	// that user message. Branching from an assistant message copies through that
	// assistant response and does not fabricate a user turn.
	visibleOrdinal := -1
	includeSelected := true
	resendText := ""
	msgCount := 0
	for i, it := range items {
		if it.Type != "msg" {
			continue
		}
		role, _ := it.Data["role"].(string)
		if role != "user" && role != "assistant" {
			continue
		}
		// Older transcripts stored replies that were only tool calls.
		if t, _ := it.Data["text"].(string); role == "assistant" && domain.StripToolCalls(t) == "" && i != displayItemIndex {
			continue
		}
		if i == displayItemIndex {
			text, _ := it.Data["text"].(string)
			visibleOrdinal = msgCount
			if role == "user" {
				includeSelected = false
				resendText = text
			}
			break
		}
		// There should be a matching SavedMessage for every display msg.
		msgCount++
	}
	cutMsgIdx := visibleMessageCut(src.Messages, visibleOrdinal, includeSelected)
	if cutMsgIdx < 0 || cutMsgIdx > len(src.Messages) {
		return nil, fmt.Errorf("could not map display item to message index")
	}

	now := time.Now()
	branch := &history.Conversation{
		ConvMeta: history.ConvMeta{
			ID:        conversationID("branch", 0),
			Title:     src.Title + " (branch)",
			WorkDir:   src.WorkDir,
			Model:     src.Model,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Messages:   make([]history.SavedMessage, cutMsgIdx),
		TokenUsage: src.TokenUsage,
	}
	copy(branch.Messages, src.Messages[:cutMsgIdx])

	// Truncate display items to match the copied history. When re-sending a user
	// message, omit that original item so the new branch has one canonical turn.
	displayCut := displayItemIndex + 1
	if resendText != "" {
		displayCut = displayItemIndex
	}
	branchItems := make([]displayItem, displayCut)
	copy(branchItems, items[:displayCut])
	raw, _ := json.Marshal(branchItems)
	branch.DisplayItems = json.RawMessage(raw)

	if err := history.Save(branch); err != nil {
		return nil, err
	}

	a.setActive(branch.ID)
	if resendText != "" {
		if resp := a.startRun(branch.ID, resendText, nil); resp.Error != "" {
			return nil, errors.New(resp.Error)
		}
	}
	return branch, nil
}

// ── Conversation Import ─────────────────────────────────────────────────────────

// ImportConversation opens a file dialog and imports conversations from:
//   - This app's own JSON format (a single Conversation JSON)
//   - ChatGPT conversations.json  (array of {title, messages: [{role, content}]})
//   - Claude claude_conversations.json (array of {name, messages: [{role, content}]})
//
// Returns the list of newly imported ConvMeta so the frontend can show a preview.
func (a *App) ImportConversation() string {
	src, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import Conversation",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON files (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil || src == "" {
		return ""
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return "Cannot read file: " + err.Error()
	}

	imported := a.importFromJSON(data)
	if imported == 0 {
		return "No valid conversations found in the file."
	}
	return fmt.Sprintf("Imported %d conversation(s).", imported)
}

func (a *App) importFromJSON(data []byte) int {
	count := 0

	// Try own format first (single Conversation object).
	var conv history.Conversation
	if err := json.Unmarshal(data, &conv); err == nil && conv.ID != "" {
		conv.ID = conversationID("import", 0)
		conv.CreatedAt = time.Now()
		conv.UpdatedAt = time.Now()
		if conv.Title == "" {
			conv.Title = "Imported conversation"
		}
		if len(conv.DisplayItems) == 0 {
			conv.DisplayItems = displayItemsFromMessages(conv.Messages)
		}
		if err := history.Save(&conv); err == nil {
			count++
		}
		return count
	}

	// Try array — might be ChatGPT or Claude format.
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return count
	}

	for i, item := range raw {
		if c := a.importRawItem(item); c != nil {
			c.ID = conversationID("import", i)
			if err := history.Save(c); err == nil {
				count++
			}
		}
	}
	return count
}

func (a *App) importRawItem(raw json.RawMessage) *history.Conversation {
	// Both supported shapes carry a messages list; they differ only in whether
	// the conversation is named "title" (ChatGPT style) or "name" (Claude style).
	var chat struct {
		Title    string `json:"title"`
		Name     string `json:"name"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &chat); err != nil || len(chat.Messages) == 0 {
		return nil
	}

	prefix, title, fallback := "chatgpt", chat.Title, "ChatGPT import"
	if title == "" && chat.Name != "" {
		prefix, title, fallback = "claude", chat.Name, "Claude import"
	}
	if title == "" {
		title = fallback
	}

	now := time.Now()
	conv := &history.Conversation{
		ConvMeta: history.ConvMeta{
			ID:        conversationID(prefix, 0),
			Title:     title,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Messages: make([]history.SavedMessage, 0, len(chat.Messages)),
	}
	for _, m := range chat.Messages {
		if m.Role == "user" || m.Role == "assistant" {
			conv.Messages = append(conv.Messages, history.SavedMessage{Role: m.Role, Content: m.Content})
		}
	}
	conv.DisplayItems = displayItemsFromMessages(conv.Messages)
	return conv
}

func conversationID(prefix string, offset int) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), offset)
}

func displayItemsFromMessages(messages []history.SavedMessage) json.RawMessage {
	items := make([]displayItem, 0, len(messages))
	for _, m := range messages {
		if (m.Role != "user" && m.Role != "assistant") || strings.HasPrefix(m.Content, "TOOL_RESULT") {
			continue
		}
		items = append(items, displayItem{
			Type: "msg",
			Data: map[string]any{"role": m.Role, "text": m.Content, "streaming": false},
		})
	}
	raw, _ := json.Marshal(items)
	return json.RawMessage(raw)
}

func visibleMessageCut(messages []history.SavedMessage, visibleOrdinal int, includeSelected bool) int {
	if visibleOrdinal < 0 {
		return -1
	}
	seen := 0
	for i, m := range messages {
		if !isVisibleSavedMessage(m) {
			continue
		}
		if seen == visibleOrdinal {
			if includeSelected {
				return i + 1
			}
			return i
		}
		seen++
	}
	return -1
}

func isVisibleSavedMessage(m history.SavedMessage) bool {
	if m.Role != "user" && m.Role != "assistant" {
		return false
	}
	if m.Role == "assistant" {
		return visibleReply(m.Content) != ""
	}
	return !strings.HasPrefix(m.Content, "TOOL_RESULT")
}

// UsageBudget is the shared account-wide usage pool: a fixed data + fund
// allowance consumed by token usage across all conversations.
type UsageBudget struct {
	Tokens       int64   `json:"tokens"`         // cumulative tokens used account-wide
	DataTotalGB  float64 `json:"data_total_gb"`  // total data allowance
	DataRemainGB float64 `json:"data_remain_gb"` // remaining data
	FundTotal    float64 `json:"fund_total"`     // total fund allowance
	FundRemain   float64 `json:"fund_remain"`    // remaining fund
	FundUnit     string  `json:"fund_unit"`      // currency label for the fund
}

// computeBudget derives the shared budget from the global token counter and
// config. One token is treated as ~4 bytes of data; the fund is charged per
// million tokens. Callers must hold a.mu.
func (a *App) computeBudget() UsageBudget {
	tokens := a.globalTokens
	dataUsedGB := float64(tokens) * 4 / 1e9
	fundUsed := float64(tokens) / 1e6 * a.cfg.FundPerMTokens
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		return v
	}
	return UsageBudget{
		Tokens:       tokens,
		DataTotalGB:  a.cfg.BudgetDataGB,
		DataRemainGB: clamp(a.cfg.BudgetDataGB - dataUsedGB),
		FundTotal:    a.cfg.BudgetFund,
		FundRemain:   clamp(a.cfg.BudgetFund - fundUsed),
		FundUnit:     "STRRIAL",
	}
}

// GetUsageBudget returns the current shared usage budget snapshot.
func (a *App) GetUsageBudget() UsageBudget {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.computeBudget()
}

// userMessageIndex returns the slice index of the ordinal-th user message
// (0-based), or -1 if there are fewer user messages than that.
func userMessageIndex(msgs []history.SavedMessage, ordinal int) int {
	seen := 0
	for i, m := range msgs {
		if m.Role != string(domain.RoleUser) {
			continue
		}
		if strings.HasPrefix(m.Content, "TOOL_RESULT") {
			continue // protocol echo, not a real user message
		}
		if seen == ordinal {
			return i
		}
		seen++
	}
	return -1
}

// truncateDisplayItems drops display items from the ordinal-th user message item
// onward, so the saved transcript matches the truncated message history.
func truncateDisplayItems(raw json.RawMessage, ordinal int) json.RawMessage {
	items := parseDisplayItems(raw)
	seen := 0
	cut := len(items)
	for i, it := range items {
		if it.Type != "msg" {
			continue
		}
		if role, _ := it.Data["role"].(string); role != "user" {
			continue
		}
		if seen == ordinal {
			cut = i
			break
		}
		seen++
	}
	out, err := json.Marshal(items[:cut])
	if err != nil {
		return raw
	}
	return json.RawMessage(out)
}

// ── Tools ─────────────────────────────────────────────────────────────────────

type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (a *App) ListTools() []ToolInfo {
	reg := tools.DefaultRegistry("/")
	all := reg.All()
	out := make([]ToolInfo, len(all))
	for i, t := range all {
		out[i] = ToolInfo{Name: t.Name(), Description: t.Description()}
	}
	return out
}

// ── Version & updates ─────────────────────────────────────────────────────────

// UpdateInfo is returned by CheckForUpdate.
type UpdateInfo struct {
	Available bool   `json:"available"`
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	URL       string `json:"url"`
}

// GetVersion returns the version string embedded at build time.
func (a *App) GetVersion() string { return Version }

// CheckForUpdate fetches the latest GitHub release and compares it with the
// current build version. Returns immediately on any network error (no crash).
func (a *App) CheckForUpdate() UpdateInfo {
	info := UpdateInfo{Current: Version}
	if Version == "dev" {
		return info // skip update checks in dev builds
	}

	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet,
		"https://api.github.com/repos/komajdon/LocalAgentKit/releases/latest", nil)
	if err != nil {
		return info
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return info
	}
	// Close the body on every status so the connection is not leaked.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return info
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || payload.TagName == "" {
		return info
	}
	info.Latest = payload.TagName
	info.URL = payload.HTMLURL
	info.Available = isNewerVersion(payload.TagName, Version)
	return info
}

// isNewerVersion returns true when latest is semantically greater than current.
// Both strings may have a leading "v".
func isNewerVersion(latest, current string) bool {
	parse := func(s string) [3]int {
		s = strings.TrimPrefix(s, "v")
		parts := strings.SplitN(s, ".", 3)
		var nums [3]int
		for i, p := range parts {
			if i >= 3 {
				break
			}
			// A non-numeric part is deliberately left as zero.
			_, _ = fmt.Sscanf(p, "%d", &nums[i])
		}
		return nums
	}
	l, c := parse(latest), parse(current)
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// ── Speech-to-text ────────────────────────────────────────────────────────────

// WhisperAvailable reports whether the whisper CLI is installed on PATH.
func (a *App) WhisperAvailable() bool {
	return whisper.Available()
}

// TranscribeAudio receives raw audio bytes (WebM/WAV), runs whisper on them,
// and returns the plain-text transcript.
// On first call the model (~74 MB for base) is downloaded automatically.
func (a *App) TranscribeAudio(audioBytes []byte) (string, error) {
	model := a.config().WhisperModel
	if model == "" {
		model = "base"
	}
	dataDir := store.DataDir()

	// Pre-download model and emit progress if it isn't cached yet.
	modelPath := whisper.ModelPath(model, dataDir)
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		runtime.EventsEmit(a.ctx, "whisper:downloading", model)
		if _, err := whisper.EnsureModel(model, dataDir); err != nil {
			runtime.EventsEmit(a.ctx, "whisper:download_error", err.Error())
			return "", err
		}
		runtime.EventsEmit(a.ctx, "whisper:ready", model)
	}

	return whisper.Transcribe(audioBytes, model, dataDir)
}

// StartRecording begins capturing microphone audio in the background.
func (a *App) StartRecording() error {
	a.recMu.Lock()
	defer a.recMu.Unlock()
	if a.recSes != nil {
		return fmt.Errorf("recording already in progress")
	}
	ses, err := recorder.Start()
	if err != nil {
		return err
	}
	a.recSes = ses
	return nil
}

// StopRecording stops the microphone, runs Whisper on the captured audio,
// and returns the transcript.
func (a *App) StopRecording() (string, error) {
	a.recMu.Lock()
	ses := a.recSes
	a.recSes = nil
	a.recMu.Unlock()

	if ses == nil {
		return "", fmt.Errorf("no active recording")
	}

	wavData, err := ses.Stop()
	if err != nil {
		return "", err
	}

	model := a.config().WhisperModel
	if model == "" {
		model = "base"
	}
	dataDir := store.DataDir()

	modelPath := whisper.ModelPath(model, dataDir)
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		runtime.EventsEmit(a.ctx, "whisper:downloading", model)
		if _, err := whisper.EnsureModel(model, dataDir); err != nil {
			runtime.EventsEmit(a.ctx, "whisper:download_error", err.Error())
			return "", err
		}
		runtime.EventsEmit(a.ctx, "whisper:ready", model)
	}

	return whisper.Transcribe(wavData, model, dataDir)
}

func (a *App) PickDirectory() string {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Working Directory",
	})
	if err != nil {
		return ""
	}
	return dir
}

// parseDisplayItems safely parses existing display items from raw JSON.
func parseDisplayItems(raw json.RawMessage) []displayItem {
	if len(raw) == 0 {
		return nil
	}
	var items []displayItem
	_ = json.Unmarshal(raw, &items)
	return items
}

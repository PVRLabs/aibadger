package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PVRLabs/aibadger/internal/handoff"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/reviewtask"
	"github.com/PVRLabs/aibadger/internal/sessionimport"
	"github.com/PVRLabs/aibadger/internal/sessionimport/codex"
	"github.com/PVRLabs/aibadger/internal/startup"
	"github.com/PVRLabs/aibadger/pkg/badger"
)

const (
	continueIDA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	continueIDB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

func chooseFirstSession(sessions []sessionimport.Summary) (string, error) { return sessions[0].ID, nil }

func continueFixture(t *testing.T, root string, at time.Time, id, cwd, message string) string {
	t.Helper()
	dir := filepath.Join(root, at.Format("2006/01/02"))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fmt.Sprintf("rollout-%s-%s.jsonl", at.Format("2006-01-02T15-04-05"), id))
	data := fmt.Sprintf("{\"type\":\"session_meta\",\"payload\":{\"id\":%q,\"cwd\":%q,\"timestamp\":%q}}\n", id, cwd, at.Format(time.RFC3339)) +
		fmt.Sprintf("{\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":%q}}\n", message) +
		"{\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"message\":\"Done 🦡\"}}\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestContinueArgumentForms(t *testing.T) {
	for _, tt := range []struct {
		args      []string
		agent, id string
		fail      bool
	}{
		{args: []string{"continue"}},
		{args: []string{"continue", "--agent", "codex"}, agent: "codex"},
		{args: []string{"continue", "--agent=codex", "--session=" + continueIDA}, agent: "codex", id: continueIDA},
		{args: []string{"continue", "--session", continueIDA}, fail: true},
		{args: []string{"continue", "--agent", "other"}, fail: true},
		{args: []string{"continue", "--agent", "codex", "--session", "../file"}, fail: true},
		{args: []string{"continue", "--agent", "codex", "--agent", "codex"}, fail: true},
		{args: []string{"continue", "--agent", "codex", "extra"}, fail: true},
		{args: []string{"continue", "--agent"}, fail: true},
	} {
		cfg := loadConfig(tt.args)
		if (cfg.parseErr != nil) != tt.fail || (!tt.fail && (cfg.continueAgent != tt.agent || cfg.continueSession != tt.id)) {
			t.Errorf("loadConfig(%v) = %+v", tt.args, cfg)
		}
	}
}

func TestContinueHandoffPrecedenceAndLaziness(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, handoff.Filename)
	write := func(data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	called := 0
	factory := func() (codex.Source, error) { called++; return codex.Source{}, errors.New("should not inspect Codex") }
	write("BADGER-HANDOFF-V1\nmode: design\n\nexisting goal")
	cfg := loadConfig([]string{"continue"})
	if err := prepareContinue(&cfg, root, false, factory, nil); err != nil {
		t.Fatal(err)
	}
	if called != 0 || cfg.startupGoal != "existing goal" || cfg.codexImport != nil {
		t.Fatalf("handoff route = %+v, factory calls=%d", cfg, called)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("valid handoff remains: %v", err)
	}
	write("malformed")
	cfg = loadConfig([]string{"continue"})
	if err := prepareContinue(&cfg, root, false, factory, nil); err == nil {
		t.Fatal("malformed handoff fell back")
	}
	if called != 0 {
		t.Fatalf("Codex factory called %d times", called)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("malformed handoff was consumed: %v", err)
	}
}

func TestContinueDirectIDBypassesHandoffAndPicker(t *testing.T) {
	root := t.TempDir()
	handoffPath := filepath.Join(root, handoff.Filename)
	if err := os.WriteFile(handoffPath, []byte("malformed"), 0600); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	sessions := filepath.Join(root, "sessions")
	path := continueFixture(t, sessions, at, continueIDA, root, "old requested task")
	before, _ := os.ReadFile(path)
	cfg := loadConfig([]string{"continue", "--agent", "codex", "--session", continueIDA})
	if err := prepareContinue(&cfg, root, false, func() (codex.Source, error) { return codex.Source{Root: sessions}, nil }, nil); err != nil {
		t.Fatal(err)
	}
	if cfg.codexImport == nil || !strings.Contains(cfg.codexImport.Text, "old requested task") || cfg.focus != protocol.FocusDesign || !cfg.literalStartup {
		t.Fatalf("Codex direct route = %+v", cfg)
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, before) {
		t.Fatalf("Codex file changed: %v", err)
	}
	if data, err := os.ReadFile(handoffPath); err != nil || string(data) != "malformed" {
		t.Fatalf("handoff changed: %q, %v", data, err)
	}
}

func TestContinuePickerAndSelectionErrors(t *testing.T) {
	root := newGitRepo(t)
	sessions := filepath.Join(root, "sessions")
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	continueFixture(t, sessions, now, continueIDA, t.TempDir(), "unrelated task")
	continueFixture(t, sessions, now.Add(-time.Second), continueIDB, root, "current task")
	factory := func() (codex.Source, error) {
		return codex.Source{Root: sessions, Now: func() time.Time { return now }}, nil
	}
	cfg := loadConfig([]string{"continue", "--agent", "codex"})
	var listed []sessionimport.Summary
	choose := func(sessions []sessionimport.Summary) (string, error) {
		listed = sessions
		return sessions[0].ID, nil
	}
	if err := prepareContinue(&cfg, root, true, factory, choose); err != nil {
		t.Fatal(err)
	}
	if cfg.continueSession != continueIDB || len(listed) != 2 || listed[0].ID != continueIDB || !listed[0].ProjectMatch || listed[1].ID != continueIDA {
		t.Fatalf("picker sessions = %+v, config=%+v", listed, cfg)
	}
	plain := loadConfig([]string{"continue"})
	if err := prepareContinue(&plain, root, true, factory, chooseFirstSession); err != nil || plain.continueSession != continueIDB {
		t.Fatalf("plain continue fallback = %+v, %v", plain, err)
	}
	cfg = loadConfig([]string{"continue", "--agent", "codex"})
	if err := prepareContinue(&cfg, root, false, factory, nil); err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("non-interactive selection = %v", err)
	}
	cfg = loadConfig([]string{"continue", "--agent", "codex"})
	if err := prepareContinue(&cfg, root, true, factory, func([]sessionimport.Summary) (string, error) {
		return "", errors.New("Codex session selection canceled")
	}); err == nil || !strings.Contains(err.Error(), "canceled") || cfg.codexImport != nil {
		t.Fatalf("canceled selection = %v", err)
	}
	empty := func() (codex.Source, error) {
		return codex.Source{Root: filepath.Join(root, "empty"), Now: func() time.Time { return now }}, nil
	}
	cfg = loadConfig([]string{"continue", "--agent", "codex"})
	if err := prepareContinue(&cfg, root, true, empty, chooseFirstSession); err == nil || !strings.Contains(err.Error(), "no recent") {
		t.Fatalf("empty selection = %v", err)
	}
	noText := filepath.Join(root, "no-text")
	path := continueFixture(t, noText, now, continueIDA, root, "task")
	if err := os.WriteFile(path, []byte(`{"type":"session_meta","payload":{"id":"`+continueIDA+`"}}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg = loadConfig([]string{"continue", "--agent", "codex", "--session", continueIDA})
	if err := prepareContinue(&cfg, root, false, func() (codex.Source, error) { return codex.Source{Root: noText}, nil }, nil); !errors.Is(err, sessionimport.ErrNoConversation) {
		t.Fatalf("empty selected session = %v", err)
	}
}

func TestContinueNonGitDirectoryHasNoCurrentRepository(t *testing.T) {
	root := filepath.Join(newGitRepo(t), "subdirectory")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	sessions := filepath.Join(root, "sessions")
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	continueFixture(t, sessions, now, continueIDA, t.TempDir(), "newer unrelated task")
	continueFixture(t, sessions, now.Add(-time.Second), continueIDB, root, "older same-directory task")
	cfg := loadConfig([]string{"continue", "--agent", "codex"})
	var listed []sessionimport.Summary
	err := prepareContinue(&cfg, root, true, func() (codex.Source, error) {
		return codex.Source{Root: sessions, Now: func() time.Time { return now }}, nil
	}, func(sessions []sessionimport.Summary) (string, error) {
		listed = sessions
		return sessions[0].ID, nil
	})
	if err != nil || cfg.continueSession != continueIDA || len(listed) != 2 || listed[0].ProjectMatch || listed[1].ProjectMatch {
		t.Fatalf("non-Git picker = %+v, config=%+v, err=%v", listed, cfg, err)
	}
}

func TestContinueNonInteractiveRejectsBeforeSourceConstruction(t *testing.T) {
	calls := 0
	cfg := loadConfig([]string{"continue", "--agent", "codex"})
	err := prepareContinue(&cfg, t.TempDir(), false, func() (codex.Source, error) {
		calls++
		return codex.Source{}, errors.New("home unavailable")
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "use --agent codex --session") || calls != 0 {
		t.Fatalf("non-interactive route: err=%v, source calls=%d", err, calls)
	}
}

func TestCodexStartupRetainsExistingAttachmentAndExactExtractedText(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	sessions := filepath.Join(root, "sessions")
	continueFixture(t, sessions, at, continueIDA, root, "Please continue 🦡")
	extracted, err := (codex.Source{Root: sessions}).Extract(continueIDA, sessionimport.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := badger.DefaultConfig()
	cfg.Root = root
	app := loadConfig([]string{"continue", "--agent", "codex", "--session", continueIDA})
	app.focus = protocol.FocusDesign
	app.focusExplicit = true
	applyInteractiveFocus(&cfg, &app)
	build := func(string, reviewtask.Options) (startup.Context, error) {
		return startup.Context{Attachments: []startup.Attachment{{Type: "text", Source: "existing", Text: "existing notes"}}}, nil
	}
	applyCodexStartupWithBuilder(&cfg, continueIDA, extracted, build)
	if cfg.Focus != protocol.FocusDesign {
		t.Fatalf("focus = %q", cfg.Focus)
	}
	if !cfg.SkipOnboarding || cfg.Startup.Goal != codexContinueGoal || !cfg.Startup.LiteralGoal {
		t.Fatalf("startup = %+v", cfg.Startup)
	}
	var imported *startup.Attachment
	for i := range cfg.Startup.Attachments {
		attachment := &cfg.Startup.Attachments[i]
		if attachment.Source == "Codex session "+continueIDA {
			imported = attachment
		}
	}
	if len(cfg.Startup.Attachments) != 2 || cfg.Startup.Attachments[0].Text != "existing notes" || imported == nil || imported.Type != "text" || imported.Text != extracted.Text || !strings.HasSuffix(imported.Text, "\n") || !strings.Contains(imported.Text, "🦡") {
		t.Fatalf("attachments = %+v", cfg.Startup.Attachments)
	}
}

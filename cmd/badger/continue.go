package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PVRLabs/aibadger/internal/handoff"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/reviewtask"
	"github.com/PVRLabs/aibadger/internal/sessionimport"
	"github.com/PVRLabs/aibadger/internal/sessionimport/codex"
	"github.com/PVRLabs/aibadger/internal/startup"
	"github.com/PVRLabs/aibadger/pkg/badger"
)

const codexContinueGoal = "Continue this task using the attached Codex session context."

func parseContinueArgs(args []string, cfg *appConfig) error {
	if len(args) == 0 {
		return nil
	}
	seenAgent, seenSession := false, false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name := ""
		switch {
		case arg == "--agent" || strings.HasPrefix(arg, "--agent="):
			name = "agent"
			if seenAgent {
				return fmt.Errorf("duplicate continue --agent")
			}
			seenAgent = true
		case arg == "--session" || strings.HasPrefix(arg, "--session="):
			name = "session"
			if seenSession {
				return fmt.Errorf("duplicate continue --session")
			}
			seenSession = true
		default:
			return fmt.Errorf("continue command does not accept flags or arguments")
		}
		value, assigned := flagValue(arg, name)
		if !assigned {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return fmt.Errorf("continue --%s requires a value", name)
			}
			i++
			value = args[i]
		}
		if value == "" {
			return fmt.Errorf("continue --%s requires a value", name)
		}
		if name == "agent" {
			cfg.continueAgent = value
		} else {
			cfg.continueSession = value
		}
	}
	if cfg.continueSession != "" && cfg.continueAgent != "codex" {
		return fmt.Errorf("continue --session requires --agent codex")
	}
	if seenAgent && cfg.continueAgent != "codex" {
		return fmt.Errorf("unsupported continue agent %q", cfg.continueAgent)
	}
	if cfg.continueSession != "" && !codex.ValidID(cfg.continueSession) {
		return fmt.Errorf("invalid Codex session ID %q", cfg.continueSession)
	}
	return nil
}

func prepareContinue(cfg *appConfig, root string, interactive bool, newSource func() (codex.Source, error), choose func([]sessionimport.Summary) (string, error)) error {
	if cfg.continueAgent == "" {
		// A present but malformed or unreadable handoff stays on the existing
		// consumer path. A race after this check also returns its current error.
		_, err := os.Lstat(filepath.Join(root, handoff.Filename))
		if err == nil {
			content, err := handoff.Consume(root)
			if err != nil {
				return err
			}
			return applyContinueContent(cfg, content)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	id := cfg.continueSession
	if id == "" && !interactive {
		return fmt.Errorf("Codex session selection requires an interactive terminal; use --agent codex --session <session-id>")
	}
	source, err := newSource()
	if err != nil {
		return err
	}
	if id == "" {
		projectRoot := ""
		if info, err := os.Lstat(filepath.Join(root, ".git")); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			projectRoot = root
		}
		sessions, err := source.List(projectRoot)
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			return fmt.Errorf("no recent Codex sessions found")
		}
		id, err = choose(sessions)
		if err != nil {
			return err
		}
	}
	conversation, err := source.Extract(id, sessionimport.Limits{})
	if err != nil {
		return fmt.Errorf("importing Codex session %s: %w", id, err)
	}
	cfg.continueSession = id
	cfg.codexImport = &conversation
	cfg.focus = protocol.FocusDesign
	cfg.focusExplicit = true
	cfg.startupGoal = codexContinueGoal
	cfg.literalStartup = true
	return nil
}

func applyCodexStartup(cfg *badger.Config, id string, conversation sessionimport.Conversation) {
	applyCodexStartupWithBuilder(cfg, id, conversation, reviewtask.BuildInteractiveContext)
}

func applyCodexStartupWithBuilder(cfg *badger.Config, id string, conversation sessionimport.Conversation, buildReview reviewContextBuilder) {
	applyHandoffStartupWithBuilder(cfg, codexContinueGoal, buildReview)
	if cfg.Startup.Status.Severity == "warning" {
		cfg.Startup.Status.Text = "Codex session loaded, but current Git context could not be prepared. Edit the goal and continue."
	} else {
		cfg.Startup.Status = startup.Status{Text: "Codex session loaded. Edit the goal before submitting.", Severity: "success"}
	}
	cfg.Startup.Attachments = append(cfg.Startup.Attachments, startup.Attachment{
		Type: "text", Source: "Codex session " + id, Text: conversation.Text,
		SizeBytes: int64(len(conversation.Text)),
	})
}

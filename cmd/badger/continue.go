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
	"github.com/PVRLabs/aibadger/internal/sessionimport/claude"
	"github.com/PVRLabs/aibadger/internal/sessionimport/codex"
	"github.com/PVRLabs/aibadger/internal/startup"
	"github.com/PVRLabs/aibadger/pkg/badger"
)

const codexContinueGoal = "Continue this task using the attached Codex session context."
const claudeContinueGoal = "Continue this task using the attached Claude session context."

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
	if cfg.continueSession != "" && cfg.continueAgent != "codex" && cfg.continueAgent != "claude" {
		return fmt.Errorf("continue --session requires --agent codex or claude")
	}
	if seenAgent && cfg.continueAgent != "codex" && cfg.continueAgent != "claude" {
		return fmt.Errorf("unsupported continue agent %q", cfg.continueAgent)
	}
	if cfg.continueSession != "" && ((cfg.continueAgent == "codex" && !codex.ValidID(cfg.continueSession)) || (cfg.continueAgent == "claude" && !claude.ValidID(cfg.continueSession))) {
		return fmt.Errorf("invalid %s session ID %q", cfg.continueAgent, cfg.continueSession)
	}
	return nil
}

func prepareContinue(cfg *appConfig, root string, interactive bool, newSource func() (codex.Source, error), choose func([]sessionimport.Summary) (string, error)) error {
	return prepareContinueWithSources(cfg, root, interactive, newSource, claude.Default, choose)
}

func prepareContinueWithSources(cfg *appConfig, root string, interactive bool, newCodex func() (codex.Source, error), newClaude func() (claude.Source, error), choose func([]sessionimport.Summary) (string, error)) error {
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
	agent := cfg.continueAgent
	if agent == "" {
		agent = "codex"
	}
	if id == "" && !interactive {
		return fmt.Errorf("%s session selection requires an interactive terminal; use --agent %s --session <session-id>", agent, agent)
	}
	var source interface {
		List(string) ([]sessionimport.Summary, error)
		Extract(string, sessionimport.Limits) (sessionimport.Conversation, error)
	}
	if agent == "claude" {
		s, err := newClaude()
		if err != nil {
			return err
		}
		source = s
	} else {
		s, err := newCodex()
		if err != nil {
			return err
		}
		source = s
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
			return fmt.Errorf("no recent %s sessions found", agent)
		}
		id, err = choose(sessions)
		if err != nil {
			return err
		}
	}
	conversation, err := source.Extract(id, sessionimport.Limits{})
	if err != nil {
		return fmt.Errorf("importing %s session %s: %w", agent, id, err)
	}
	cfg.continueSession = id
	if agent == "claude" {
		cfg.claudeImport = &conversation
	} else {
		cfg.codexImport = &conversation
	}
	cfg.focus = protocol.FocusDesign
	cfg.focusExplicit = true
	if agent == "claude" {
		cfg.startupGoal = claudeContinueGoal
	} else {
		cfg.startupGoal = codexContinueGoal
	}
	cfg.literalStartup = true
	return nil
}

func applyCodexStartup(cfg *badger.Config, id string, conversation sessionimport.Conversation) {
	applyCodexStartupWithBuilder(cfg, id, conversation, reviewtask.BuildInteractiveContext)
}

func applyCodexStartupWithBuilder(cfg *badger.Config, id string, conversation sessionimport.Conversation, buildReview reviewContextBuilder) {
	applySessionStartupWithBuilder(cfg, "Codex", codexContinueGoal, id, conversation, buildReview)
}

func applyClaudeStartup(cfg *badger.Config, id string, conversation sessionimport.Conversation) {
	applySessionStartupWithBuilder(cfg, "Claude", claudeContinueGoal, id, conversation, reviewtask.BuildInteractiveContext)
}

func applySessionStartupWithBuilder(cfg *badger.Config, agent, goal, id string, conversation sessionimport.Conversation, buildReview reviewContextBuilder) {
	applyHandoffStartupWithBuilder(cfg, goal, buildReview)
	if cfg.Startup.Status.Severity == "warning" {
		cfg.Startup.Status.Text = agent + " session loaded, but current Git context could not be prepared. Edit the goal and continue."
	} else {
		cfg.Startup.Status = startup.Status{Text: agent + " session loaded. Edit the goal before submitting.", Severity: "success"}
	}
	cfg.Startup.Attachments = append(cfg.Startup.Attachments, startup.Attachment{
		Type: "text", Source: agent + " session " + id, Text: conversation.Text,
		SizeBytes: int64(len(conversation.Text)),
	})
}

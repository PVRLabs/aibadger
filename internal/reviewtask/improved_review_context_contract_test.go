package reviewtask

import (
	"encoding/json"
	"os"
	"testing"
)

type improvedReviewContextContract struct {
	Version        int `json:"version"`
	InitialPayload struct {
		MandatoryDiff        bool   `json:"mandatory_diff"`
		Topology             string `json:"topology"`
		MaxPayloadBytes      int    `json:"max_payload_bytes"`
		MaxOptionalFileBytes int    `json:"max_optional_file_bytes"`
		MandatoryOverflow    string `json:"mandatory_overflow"`
	} `json:"initial_payload"`
	Modes []struct {
		Mode                           string `json:"mode"`
		AuthoritativeDiff              string `json:"authoritative_diff"`
		IncludesUntrackedFullAdditions bool   `json:"includes_untracked_full_additions"`
		SelectedUntracked              string `json:"selected_untracked"`
		RepositoryUntracked            string `json:"repository_untracked"`
		AcceptsSelectedPaths           bool   `json:"accepts_selected_paths"`
	} `json:"modes"`
	OptionalFilePolicy []struct {
		Change   string `json:"change"`
		FullFile string `json:"full_file"`
	} `json:"optional_file_policy"`
	SelectedPaths struct {
		Form                 string `json:"form"`
		DeletedChangedPath   string `json:"deleted_changed_path"`
		MissingUnchangedPath string `json:"missing_unchanged_path"`
		Traversal            string `json:"traversal"`
	} `json:"selected_paths"`
	RepositoryUntracked struct {
		Content           string `json:"content"`
		SensitivePaths    string `json:"sensitive_paths"`
		MaxPaths          int    `json:"max_paths"`
		OmittedCount      bool   `json:"omitted_count"`
		ExplicitSelection string `json:"explicit_selection"`
	} `json:"repository_untracked"`
	SupportingContentSource string `json:"supporting_content_source"`
	Generation              string `json:"generation"`
	PromptIntent            string `json:"prompt_intent"`
}

func TestImprovedReviewContextContractFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/improved_review_context_contract.json")
	if err != nil {
		t.Fatalf("read contract fixture: %v", err)
	}

	var contract improvedReviewContextContract
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatalf("parse contract fixture: %v", err)
	}

	if contract.Version != 3 {
		t.Fatalf("contract version = %d, want 3", contract.Version)
	}
	if !contract.InitialPayload.MandatoryDiff || contract.InitialPayload.MandatoryOverflow != "fail" {
		t.Fatalf("mandatory diff policy = %+v, want complete-or-fail", contract.InitialPayload)
	}
	if contract.InitialPayload.Topology != "omitted" {
		t.Fatalf("topology policy = %q, want omitted", contract.InitialPayload.Topology)
	}
	if contract.InitialPayload.MaxPayloadBytes != 512*1024 {
		t.Fatalf("max payload bytes = %d, want %d", contract.InitialPayload.MaxPayloadBytes, 512*1024)
	}
	if contract.InitialPayload.MaxOptionalFileBytes != 64*1024 {
		t.Fatalf("max optional file bytes = %d, want %d", contract.InitialPayload.MaxOptionalFileBytes, 64*1024)
	}

	wantModes := map[string]struct {
		diff                       string
		selected                   bool
		includesUntrackedAdditions bool
		selectedUntracked          string
		repositoryUntracked        string
	}{
		"default": {
			diff:                       "HEAD_to_worktree",
			selected:                   true,
			includesUntrackedAdditions: true,
			selectedUntracked:          "bounded_complete_text",
			repositoryUntracked:        "ranked_paths_with_bounded_complete_text",
		},
		"staged": {diff: "HEAD_to_index"},
		"branch": {diff: "merge_base_to_HEAD"},
		"commit": {diff: "requested_commit"},
	}
	if len(contract.Modes) != len(wantModes) {
		t.Fatalf("mode count = %d, want %d", len(contract.Modes), len(wantModes))
	}
	for _, mode := range contract.Modes {
		want, ok := wantModes[mode.Mode]
		if !ok {
			t.Fatalf("unexpected mode %q", mode.Mode)
		}
		if mode.AuthoritativeDiff != want.diff ||
			mode.AcceptsSelectedPaths != want.selected ||
			mode.IncludesUntrackedFullAdditions != want.includesUntrackedAdditions ||
			mode.SelectedUntracked != want.selectedUntracked ||
			mode.RepositoryUntracked != want.repositoryUntracked {
			t.Fatalf("mode %q = %+v, want %+v", mode.Mode, mode, want)
		}
		delete(wantModes, mode.Mode)
	}

	optionalPolicy := make(map[string]string, len(contract.OptionalFilePolicy))
	for _, policy := range contract.OptionalFilePolicy {
		optionalPolicy[policy.Change] = policy.FullFile
	}
	if got := optionalPolicy["untracked"]; got != "eligible_complete_worktree_addition" {
		t.Fatalf("untracked optional-file policy = %q, want eligible_complete_worktree_addition", got)
	}

	if contract.SelectedPaths.Form != "repository_relative_literal" ||
		contract.SelectedPaths.DeletedChangedPath != "accept" ||
		contract.SelectedPaths.MissingUnchangedPath != "reject_stale" ||
		contract.SelectedPaths.Traversal != "reject" {
		t.Fatalf("selected-path policy is incomplete: %+v", contract.SelectedPaths)
	}
	if contract.RepositoryUntracked.Content != "bounded_complete_text" ||
		contract.RepositoryUntracked.SensitivePaths != "omitted" ||
		contract.RepositoryUntracked.MaxPaths != 25 ||
		!contract.RepositoryUntracked.OmittedCount ||
		contract.RepositoryUntracked.ExplicitSelection != "bounded_complete_text" {
		t.Fatalf("repository-untracked policy is incomplete: %+v", contract.RepositoryUntracked)
	}
	if contract.SupportingContentSource != "current_checked_out_worktree" {
		t.Fatalf("supporting content source = %q", contract.SupportingContentSource)
	}
	if contract.Generation != "one_repository_inspection_per_explicit_request" {
		t.Fatalf("generation timing = %q", contract.Generation)
	}
	if contract.PromptIntent != "report_findings_now_request_selectors_only_if_needed" {
		t.Fatalf("prompt intent = %q", contract.PromptIntent)
	}
}

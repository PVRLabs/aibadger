package codex

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PVRLabs/aibadger/internal/sessionimport"
)

const (
	idA = "11111111-1111-1111-1111-111111111111"
	idB = "22222222-2222-2222-2222-222222222222"
	idC = "33333333-3333-3333-3333-333333333333"
)

func fixture(t *testing.T, root string, day time.Time, id, cwd, extra string) string {
	t.Helper()
	dir := bucket(root, day)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("rollout-%s-%s.jsonl", day.Format("2006-01-02T15-04-05"), id)
	path := filepath.Join(dir, name)
	content := fmt.Sprintf("{\"type\":\"session_meta\",\"payload\":{\"id\":%q,\"timestamp\":%q,\"cwd\":%q}}\n", id, day.Format(time.RFC3339), cwd) + extra
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func user(text string) string {
	return fmt.Sprintf("{\"type\":\"event_msg\",\"payload\":{\"type\":\"user_message\",\"message\":%q}}\n", text)
}

func TestListRecentRankingBoundsAndNoMatches(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	repo := filepath.Join(root, "repo")
	s := Source{Root: filepath.Join(root, "sessions"), Now: func() time.Time { return now }}
	if got, err := s.List(repo); err != nil || len(got) != 0 {
		t.Fatalf("empty list = %+v, %v", got, err)
	}
	fixture(t, s.Root, now, idA, filepath.Join(repo, "subdir"), user("current project"))
	fixture(t, s.Root, now.Add(-time.Second), idB, filepath.Join(root, "other"), user("unrelated"))
	fixture(t, s.Root, now.AddDate(0, 0, -16), idC, repo, user("old"))
	got, err := s.List(repo)
	if err != nil || len(got) != 2 {
		t.Fatalf("list = %+v, %v", got, err)
	}
	if got[0].ID != idA || !got[0].ProjectMatch || got[0].Label != "current project" || got[1].ProjectMatch {
		t.Fatalf("ranking = %+v", got)
	}
	// The first dated bucket has two entries, so an entry budget of one
	// cannot inspect the second or an older bucket.
	s.listBudget = 1
	got, _ = s.List(repo)
	if len(got) != 1 {
		t.Fatalf("bounded list = %+v", got)
	}
}

func TestResolveOlderExactIDAndInvalidInput(t *testing.T) {
	root := t.TempDir()
	s := Source{Root: root, Now: func() time.Time { return time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC) }}
	old := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	path := fixture(t, root, old, idA, root, user("old conversation"))
	got, err := s.Resolve(idA)
	if err != nil || got != path {
		t.Fatalf("resolve = %q, %v", got, err)
	}
	for _, id := range []string{"../" + idA, path, "bad", strings.ToUpper(idA) + ".jsonl"} {
		if _, err := s.Resolve(id); err == nil {
			t.Errorf("accepted %q", id)
		}
	}
	if _, err := s.Resolve(idB); !errors.Is(err, sessionimport.ErrNotFound) {
		t.Fatalf("missing = %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(idA); !errors.Is(err, sessionimport.ErrNotFound) {
		t.Fatalf("removed = %v", err)
	}
}

func TestResolveIncompleteAndRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	s := Source{Root: root, lookupBudget: 3}
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("%08d-1111-1111-1111-111111111111", i)
		fixture(t, root, time.Date(2026, 9, 22, 0, 0, i, 0, time.UTC), id, root, user("x"))
	}
	if _, err := s.Resolve(idA); !errors.Is(err, sessionimport.ErrLookupIncomplete) {
		t.Fatalf("limited lookup = %v", err)
	}
	other := fixture(t, root, time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC), idB, root, user("x"))
	link := filepath.Join(bucket(root, time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)), "rollout-2026-09-22T00-00-00-"+idC+".jsonl")
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	s.lookupBudget = 0
	if _, err := s.Resolve(idC); !errors.Is(err, sessionimport.ErrNotFound) {
		t.Fatalf("symlink lookup = %v", err)
	}
}

func TestListStableTiesAndNonRegularEntries(t *testing.T) {
	root := t.TempDir()
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	fixture(t, root, day, idB, root, user("second"))
	fixture(t, root, day, idA, root, user("first"))
	outside := filepath.Join(t.TempDir(), "other.jsonl")
	if err := os.WriteFile(outside, []byte(user("outside")), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bucket(root, day), "rollout-2026-09-22T00-00-00-"+idC+".jsonl")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	s := Source{Root: root, Now: func() time.Time { return day }}
	got, err := s.List(root)
	if err != nil || len(got) != 2 || got[0].ID != idA || got[1].ID != idB {
		t.Fatalf("stable list = %+v, %v", got, err)
	}
	got, err = s.List(root)
	if err != nil || len(got) != 2 || got[0].ID != idA {
		t.Fatalf("repeat list = %+v, %v", got, err)
	}
}

func TestListRanksProjectMatchesBeforeRecencyAfterCandidateInspection(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	repo := filepath.Join(root, "repo")
	for i := 0; i < listCandidateLimit+1; i++ {
		id := fmt.Sprintf("%08d-1111-1111-1111-111111111111", i)
		cwd := filepath.Join(root, "unrelated")
		if i == 10 || i == listCandidateLimit {
			cwd = repo
		}
		fixture(t, root, now.Add(-time.Duration(i)*time.Second), id, cwd, user("task"))
	}
	s := Source{Root: root, Now: func() time.Time { return now }}
	got, err := s.List(repo)
	if err != nil || len(got) != listCandidateLimit {
		t.Fatalf("list = %d entries, %v", len(got), err)
	}
	wantMatch := fmt.Sprintf("%08d-1111-1111-1111-111111111111", 10)
	if got[0].ID != wantMatch || !got[0].ProjectMatch {
		t.Fatalf("newer project match missing: %+v", got[0])
	}
	oldestMatch := fmt.Sprintf("%08d-1111-1111-1111-111111111111", listCandidateLimit)
	if got[1].ID != oldestMatch || !got[1].ProjectMatch {
		t.Fatalf("older project match excluded before ranking: %+v", got[1])
	}
	s.inspectionBudget = 10
	got, err = s.List(repo)
	if err != nil || len(got) != 10 || got[0].ProjectMatch {
		t.Fatalf("inspection bound = %+v, %v", got, err)
	}
}

func TestDefaultRejectsUnavailableHome(t *testing.T) {
	s, err := sourceFromHome("", errors.New("home unavailable"))
	if err == nil || s.Root != "" {
		t.Fatalf("default source = %+v, %v", s, err)
	}
	if _, err := s.List(""); err == nil {
		t.Fatal("zero source listed a relative sessions directory")
	}
	s, err = sourceFromHome("relative-home", nil)
	if err == nil || s.Root != "" {
		t.Fatalf("relative home source = %+v, %v", s, err)
	}
}

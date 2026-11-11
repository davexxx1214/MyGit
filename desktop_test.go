package main

import (
	"context"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"mygit/internal/git"
	loc "mygit/locales"
)

func desktopPreferences(t *testing.T) {
	t.Helper()
	old := prefs
	copy := *prefs
	copy.path = ""
	copy.Repositories, copy.ActiveRepository = nil, ""
	copy.SidebarHidden, copy.DesktopLog = false, false
	copy.SidebarW, copy.LogH, copy.RepositoriesH = 280, 390, 280
	copy.FoldedBranches = map[string]bool{}
	prefs = &copy
	t.Cleanup(func() { prefs = old })
}

func TestDesktopRepositoryLibrary(t *testing.T) {
	desktopPreferences(t)
	first, second := testRepo(t), testRepo(t) // unrelated folders, not one scanned root
	registerRepository(first.Dir)
	registerRepository(second.Dir)
	registerRepository(filepath.Join(first.Dir, "."))
	if len(prefs.Repositories) != 2 {
		t.Fatalf("duplicates: %v", prefs.Repositories)
	}
	ws, tt, settle := startWorkspace(&workspace{managed: true}, []*git.Repo{first, second}, func(view func(*ui.Context)) *ui.Tester { return ui.NewTester(view, 1400, 900) })
	for _, text := range []string{loc.Repositories(), loc.BranchesLocal(), loc.Commit(), "Log", loc.Discard()} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q: %v", text, tt.Texts())
		}
	}
	ws.apps[0].message = "draft for first"
	if err := tt.Click(second.Dir); err != nil {
		t.Fatal(err)
	}
	settle()
	ws.apps[1].message = "draft for second"
	if err := tt.Click(first.Dir); err != nil {
		t.Fatal(err)
	}
	settle()
	if ws.active != 0 || ws.apps[0].message != "draft for first" || ws.apps[1].message != "draft for second" {
		t.Fatal("repository drafts were lost")
	}
	if err := tt.RightClick(second.Dir); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), loc.RemoveRepository()) {
		t.Fatalf("menu %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem(loc.RemoveRepository()); err != nil {
		t.Fatal(err)
	}
	settle()
	if len(ws.apps) != 1 || len(prefs.Repositories) != 1 {
		t.Fatalf("remove: %d %v", len(ws.apps), prefs.Repositories)
	}
	if _, err := os.Stat(filepath.Join(second.Dir, "notes.txt")); err != nil {
		t.Fatalf("removing a path touched its files: %v", err)
	}
	ws.removeRepository(first.Dir)
	settle()
	if !tt.HasText(loc.RepositoriesEmpty()) {
		t.Fatalf("no empty state: %v", tt.Texts())
	}
	// Persist and reload a path without opening or scanning it.
	registerRepository(second.Dir)
	prefs.ActiveRepository = second.Dir
	prefs.path = filepath.Join(t.TempDir(), "settings.json")
	prefs.save()
	data, err := os.ReadFile(prefs.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved settings
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(saved.Repositories, []string{second.Dir}) || saved.ActiveRepository != second.Dir {
		t.Fatalf("saved %+v", saved)
	}
}

func TestDesktopViewsAndBranchMenus(t *testing.T) {
	desktopPreferences(t)
	r := testRepo(t)
	run(t, r.Dir, "remote", "add", "origin", r.Dir)
	run(t, r.Dir, "update-ref", "refs/remotes/origin/feature", "feature")
	registerRepository(r.Dir)
	ws, tt, settle := startWorkspace(&workspace{managed: true}, []*git.Repo{r}, func(view func(*ui.Context)) *ui.Tester { return ui.NewTester(view, 1400, 900) })
	a := ws.apps[0]
	if err := tt.Click(loc.LogView()); err != nil {
		t.Fatal(err)
	}
	settle()
	if !a.desktopLog || !tt.HasText("Merge the feature") {
		t.Fatal("Log did not show history")
	}
	for _, name := range []string{"feature", "origin/feature"} {
		if err := tt.RightClick(name); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(tt.Menu(), loc.MergeInto("main")) {
			t.Fatalf("branch %s: %v", name, tt.Menu())
		}
		tt.CloseMenu()
		tt.Frame()
	}
	if err := tt.Click(loc.CommitView()); err != nil {
		t.Fatal(err)
	}
	settle()
	if a.desktopLog || !tt.HasText("notes.txt") {
		t.Fatal("Commit did not show changes")
	}
	if err := tt.Click(loc.Discard()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(loc.DiscardAll()); err != nil {
		t.Fatal(err)
	}
	settle()
	if !a.confirm.open {
		t.Fatal("Discard bypassed confirmation")
	}
	if _, err := os.Stat(filepath.Join(r.Dir, "notes.txt")); err != nil {
		t.Fatal("discarded before confirmation")
	}
	a.confirm.open = false
	if out := os.Getenv("MYGIT_DESKTOP_DUMP"); out != "" {
		tt.SetDark(true)
		tt.Frame()
		f, err := os.Create(filepath.Join(out, "desktop-commit.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		f.Close()
		a.setDesktopLog(true)
		settle()
		f, err = os.Create(filepath.Join(out, "desktop-log.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	// Use the actual desktop controls to stage, type a message and commit.
	a.setDesktopLog(false)
	settle()
	if err := tt.Click(loc.StageFile("notes.txt")); err != nil {
		t.Fatal(err)
	}
	settle()
	if err := tt.Click(loc.Message()); err != nil {
		t.Fatal(err)
	}
	tt.Type("Commit from desktop")
	tt.Frame()
	if err := tt.Click(loc.LogView()); err != nil {
		t.Fatal(err)
	}
	settle()
	if err := tt.Click(loc.CommitView()); err != nil {
		t.Fatal(err)
	}
	settle()
	if a.message != "Commit from desktop" {
		t.Fatalf("draft lost switching views: %q", a.message)
	}
	if err := tt.Click(loc.CommitFiles(1)); err != nil {
		t.Fatal(err)
	}
	settle()
	if a.errMsg != "" || len(a.staged) != 0 || a.message != "" {
		t.Fatalf("commit failed: %s", a.errMsg)
	}
	if got, err := r.Git(context.Background(), "log", "-1", "--format=%s"); err != nil || strings.TrimSpace(got) != "Commit from desktop" {
		t.Fatalf("commit: %q %v", got, err)
	}
}

func TestRevealCommitOutsideCurrentHistory(t *testing.T) {
	tt := newTester(t)
	a := tt.a
	run(t, a.repo.Dir, "branch", "outside")
	run(t, a.repo.Dir, "switch", "outside")
	run(t, a.repo.Dir, "commit", "--allow-empty", "-m", "Outside current branch")
	hash, err := a.repo.Git(context.Background(), "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	run(t, a.repo.Dir, "switch", "main")
	a.refresh()
	tt.settle()
	a.search = "Outside"
	tt.Frame()
	if len(a.matches) != 0 {
		t.Fatal("commit unexpectedly in main's history")
	}
	// The first request becomes obsolete while history is still streaming.
	a.revealCommit("0000000000000000000000000000000000000000")
	a.revealCommit(strings.TrimSpace(hash))
	tt.settle()
	if a.scope != (git.Scope{}) || a.search != "" || a.pendingReveal != "" || a.errMsg != "" {
		t.Fatalf("navigation failed: scope=%+v pending=%s error=%s", a.scope, a.pendingReveal, a.errMsg)
	}
	if a.detail.commit == nil || a.detail.commit.Subject != "Outside current branch" {
		t.Fatalf("selected %+v", a.detail.commit)
	}
	if a.logSel < 0 || a.commits[a.logSel].Hash != strings.TrimSpace(hash) {
		t.Fatal("history row not selected")
	}
	// A branch selection cancels a pending reveal, rather than stealing focus later.
	a.revealCommit("0000000000000000000000000000000000000000")
	a.setScope(git.Scope{Ref: "HEAD"})
	tt.settle()
	if a.pendingReveal != "" || a.errMsg != "" {
		t.Fatalf("obsolete reveal: %s %s", a.pendingReveal, a.errMsg)
	}
}

// Exercise the journal in the working-tree view, then follow its selection
// through the relocated file list and the independent middle diff pane.
func TestDesktopJournalNavigation(t *testing.T) {
	desktopPreferences(t)
	r := testRepo(t)
	registerRepository(r.Dir)
	ws, tt, settle := startWorkspace(&workspace{managed: true}, []*git.Repo{r}, func(view func(*ui.Context)) *ui.Tester { return ui.NewTester(view, 1400, 1000) })
	a := ws.apps[0]
	file, ok := tt.Find("notes.txt")
	if !ok {
		t.Fatal("working files missing")
	}
	empty, ok := tt.Find(loc.DetailEmpty())
	if !ok {
		t.Fatal("middle diff pane missing")
	}
	journal, ok := tt.Find(loc.History())
	if !ok {
		t.Fatal("journal hidden in commit mode")
	}
	if file.Y >= empty.Y || empty.Y >= journal.Y {
		t.Fatalf("pane order: files=%v diff=%v journal=%v", file, empty, journal)
	}
	if err := tt.Click("Add the feature"); err != nil {
		t.Fatal(err)
	}
	settle()
	if !a.desktopLog || a.detail.commit == nil || a.detail.diff == nil {
		t.Fatal("journal selection did not open files and diff")
	}
	file, ok = tt.Find("feature.go")
	if !ok {
		t.Fatal("commit files missing from upper pane")
	}
	header, ok := tt.Find(a.detail.commit.Short())
	if !ok {
		t.Fatal("commit details missing")
	}
	journal, _ = tt.Find(loc.History())
	if file.Y >= header.Y || header.Y >= journal.Y {
		t.Fatalf("commit pane order: files=%v details=%v journal=%v", file, header, journal)
	}
	if err := tt.Click(loc.CommitView()); err != nil {
		t.Fatal(err)
	}
	settle()
	if !tt.HasText("notes.txt") || !tt.HasText(loc.History()) || a.detail.commit != nil {
		t.Fatal("return to working tree lost the files or journal")
	}
	if err := tt.Click("Add the feature"); err != nil {
		t.Fatal(err)
	}
	settle()
	if a.detail.commit == nil || a.detail.diff == nil {
		t.Fatal("reselecting the same journal entry did not restore its diff")
	}
	if err := tt.Click(loc.CommitView()); err != nil {
		t.Fatal(err)
	}
	settle()
	if out := os.Getenv("MYGIT_DESKTOP_DUMP"); out != "" {
		tt.SetDark(true)
		tt.Frame()
		f, err := os.Create(filepath.Join(out, "desktop-journal.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		f.Close()
		if err := tt.Click("Add the feature"); err != nil {
			t.Fatal(err)
		}
		settle()
		if a.detail.commit == nil || a.detail.diff == nil {
			t.Fatal("reselecting the same journal entry did not restore its diff")
		}
		f, err = os.Create(filepath.Join(out, "desktop-journal-commit.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}

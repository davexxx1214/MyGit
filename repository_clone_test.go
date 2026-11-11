package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"mygit/internal/git"
	loc "mygit/locales"
)

func TestRepositoryAddMenu(t *testing.T) {
	desktopPreferences(t)
	r := testRepo(t)
	registerRepository(r.Dir)
	ws, tt, settle := startWorkspace(&workspace{managed: true}, []*git.Repo{r}, func(view func(*ui.Context)) *ui.Tester { return ui.NewTester(view, 1200, 900) })
	if err := tt.Click(loc.AddRepository()); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Contains(menu, loc.AddRepositoryFolder()) || !slices.Contains(menu, loc.CloneMenu()) {
		t.Fatalf("add menu: %v", menu)
	}
	if err := tt.ChooseMenuItem(loc.CloneMenu()); err != nil {
		t.Fatal(err)
	}
	settle()
	if !ws.clone.open || !tt.HasText(loc.CloneTitle()) || !tt.HasText(loc.CloneURL()) {
		t.Fatalf("clone form: %v", tt.Texts())
	}
	if err := tt.Click(loc.Cancel()); err != nil {
		t.Fatal(err)
	}
	settle()
	if ws.clone.open {
		t.Fatal("cancel did not close clone")
	}
	ws.removeRepository(r.Dir)
	settle()
	// The shortcut and clone dialog also work with no active repository.
	tt.Key(ui.Cmd|ui.Shift, ui.KeyO)
	tt.Frame()
	if !ws.clone.open {
		t.Fatal("empty library cannot clone")
	}
	if err := tt.Click(loc.Cancel()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

func cloneLibraryTester(t *testing.T) (*workspace, *ui.Tester, func()) {
	t.Helper()
	queue := make(chan func(), 256)
	ws := &workspace{managed: true, updFn: func(fn func()) { queue <- fn }}
	t.Cleanup(ws.clone.stop)
	tt := ui.NewTester(ws.view, 1200, 900)
	tt.Frame()
	pump := func() {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for {
			tt.Frame()
			busy := ws.clone.running || ws.opening
			for _, a := range ws.apps {
				busy = busy || a.refreshing || a.loading || a.detail.loading
			}
			select {
			case fn := <-queue:
				fn()
				continue
			default:
			}
			if !busy {
				return
			}
			select {
			case fn := <-queue:
				fn()
			case <-time.After(time.Until(deadline)):
				t.Fatal("clone did not settle")
			}
		}
	}
	return ws, tt, pump
}

func TestLibraryCloneAndRetry(t *testing.T) {
	desktopPreferences(t)
	source := testRepo(t)
	parent := t.TempDir()
	ws, tt, pump := cloneLibraryTester(t)
	tt.Key(ui.Cmd|ui.Shift, ui.KeyO)
	tt.Frame()
	ws.clone.parent = parent
	if err := tt.Click(loc.CloneURL()); err != nil {
		t.Fatal(err)
	}
	tt.Type(source.Dir)
	tt.Frame()
	if ws.clone.name == "" {
		t.Fatal("URL did not supply a folder name")
	}
	if err := tt.Click(loc.FolderName()); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("copy")
	tt.Frame()
	// An occupied target must stay intact and must not enter the catalog.
	destination := filepath.Join(parent, "copy")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(loc.Clone()); err != nil {
		t.Fatal(err)
	}
	pump()
	if ws.clone.fail == "" || !ws.clone.open || ws.clone.running || len(prefs.Repositories) > 0 {
		t.Fatalf("clone failure: %+v", ws.clone)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "keep.txt")); err != nil || string(data) != "keep" {
		t.Fatal("failed clone changed existing files")
	}
	// Retry through the same form, without losing the source or destination parent.
	if err := tt.Click(loc.FolderName()); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("retry")
	tt.Frame()
	if err := tt.Click(loc.Clone()); err != nil {
		t.Fatal(err)
	}
	pump()
	want := filepath.Join(parent, "retry")
	if ws.clone.open || ws.clone.fail != "" || len(ws.apps) != 1 || repositoryIndex(prefs.Repositories, want) < 0 || repositoryKey(prefs.ActiveRepository) != repositoryKey(want) {
		t.Fatalf("clone not registered: %s %v", ws.clone.fail, prefs.Repositories)
	}
	if ws.apps[0].repo.Name() != "retry" || !tt.HasText("retry") {
		t.Fatal("new clone was not selected")
	}
	head, err := ws.apps[0].repo.Git(context.Background(), "rev-parse", "HEAD")
	original, _ := source.Git(context.Background(), "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(head) != strings.TrimSpace(original) {
		t.Fatalf("wrong clone: %q %v", head, err)
	}
}

func TestCloneCredentialRouting(t *testing.T) {
	old := workspaces
	t.Cleanup(func() { workspaces = old })
	ws := &workspace{managed: true, clone: clonePlan{open: true, running: true, parent: t.TempDir()}}
	workspaces = map[string]*workspace{managedWorkspaceKey: ws}
	if cloningWorkspace(filepath.Join(ws.clone.parent, ".")) != ws {
		t.Fatal("clone prompt not routed by its parent")
	}
	if cloningWorkspace(filepath.Join(ws.clone.parent, "other")) != nil {
		t.Fatal("another operation was routed to the clone")
	}
	ws.clone.running = false
	if cloningWorkspace(ws.clone.parent) != nil {
		t.Fatal("completed clone captured another prompt")
	}
}

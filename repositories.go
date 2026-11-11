package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"mygit/internal/git"
	loc "mygit/locales"
)

const managedWorkspaceKey = "mygit:repository-library"

func (ws *workspace) update(fn func()) {
	if ws.updFn != nil {
		ws.updFn(fn)
		return
	}
	ws.win.Update(fn)
}

func (ws *workspace) addingRepository() bool { return ws.opening || ws.clone.open }

func (ws *workspace) addRepositoryMenu(m *ui.Menu) {
	if m.Item(loc.AddRepositoryFolder()).Shortcut(ui.Cmd, ui.KeyO).Disabled(ws.addingRepository()).Chosen() {
		ws.pickRepository()
	}
	if m.Item(loc.CloneMenu()).Shortcut(ui.Cmd|ui.Shift, ui.KeyO).Disabled(ws.addingRepository()).Chosen() {
		ws.openClone()
	}
}

func (ws *workspace) openClone() {
	if ws.addingRepository() {
		return
	}
	parent := defaultParent()
	if len(ws.apps) > 0 {
		parent = filepath.Dir(ws.apps[ws.active].repo.Dir)
	}
	ws.clone = clonePlan{open: true, parent: parent}
}

// Clone invokes askpass from its destination's parent, before the new
// repository exists. Route those prompts to this library's clone dialog.
func cloningWorkspace(dir string) *workspace {
	for _, ws := range workspaces {
		if ws.managed && ws.clone.running && repositoryKey(ws.clone.parent) == repositoryKey(dir) {
			return ws
		}
	}
	return nil
}

// repositoryKey compares local paths without losing the path's spelling.
// Git.Open supplies canonical working-tree roots, including for subfolders.
func repositoryKey(dir string) string {
	dir = filepath.Clean(dir)
	if runtime.GOOS == "windows" {
		dir = strings.ToLower(dir)
	}
	return dir
}

func repositoryIndex(paths []string, dir string) int {
	key := repositoryKey(dir)
	return slices.IndexFunc(paths, func(p string) bool { return repositoryKey(p) == key })
}

func registerRepository(dir string) {
	if repositoryIndex(prefs.Repositories, dir) < 0 {
		prefs.Repositories = append(prefs.Repositories, dir)
	}
}

// openManaged keeps unrelated local repositories in one window. Stored
// paths are opened lazily, so missing/offline disks do not block startup.
func openManaged(repos []*git.Repo) {
	for _, r := range repos {
		registerRepository(r.Dir)
		remember("", r.Dir)
	}
	if len(repos) > 0 {
		prefs.ActiveRepository = repos[len(repos)-1].Dir
	}
	prefs.save()
	wasOpen := workspaces[managedWorkspaceKey] != nil
	openWorkspace("", managedWorkspaceKey, repos)
	ws := workspaces[managedWorkspaceKey]
	if !wasOpen {
		for _, a := range ws.apps {
			a.desktopLog = prefs.DesktopLog
		}
		if len(repos) > 0 {
			ws.activate(len(ws.apps) - 1)
		} else if len(prefs.Repositories) > 0 {
			dir := prefs.ActiveRepository
			if repositoryIndex(prefs.Repositories, dir) < 0 {
				dir = prefs.Repositories[0]
			}
			ws.selectRepository(dir)
		}
	}
}

func (ws *workspace) addManagedRepos(repos []*git.Repo) {
	for _, r := range repos {
		registerRepository(r.Dir)
		i := slices.IndexFunc(ws.apps, func(a *app) bool { return repositoryKey(a.repo.Dir) == repositoryKey(r.Dir) })
		if i < 0 {
			a := newApp(r)
			a.ws, a.win, a.desktopLog = ws, ws.win, prefs.DesktopLog
			a.update = ws.update
			ws.apps = append(ws.apps, a)
			i = len(ws.apps) - 1
			a.start()
		}
		ws.activate(i)
		prefs.ActiveRepository = r.Dir
	}
	ws.catalogErr = ""
	prefs.save()
}

func (ws *workspace) selectRepository(dir string) {
	if ws.opening {
		return
	}
	for i, a := range ws.apps {
		if repositoryKey(a.repo.Dir) == repositoryKey(dir) {
			ws.catalogErr = ""
			ws.activate(i)
			return
		}
	}
	ws.opening, ws.catalogErr = true, ""
	go func() {
		r, err := git.Open(dir)
		ws.update(func() {
			ws.opening = false
			if err != nil {
				ws.catalogErr = fmt.Sprintf("%s: %v", dir, err)
				return
			}
			// A saved path can become an alias after a directory is moved.
			if i := repositoryIndex(prefs.Repositories, dir); i >= 0 {
				if other := repositoryIndex(prefs.Repositories, r.Dir); other >= 0 && other != i {
					prefs.Repositories = slices.Delete(prefs.Repositories, i, i+1)
				} else {
					prefs.Repositories[i] = r.Dir
				}
			}
			ws.addManagedRepos([]*git.Repo{r})
		})
	}()
}

func (ws *workspace) pickRepository() {
	if ws.addingRepository() {
		return
	}
	ws.opening = true
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: ws.win, Title: loc.AddRepositoryFolder(), Directory: true})
		var r *git.Repo
		if err == nil && len(paths) > 0 {
			r, err = git.Open(paths[0])
		}
		ws.update(func() {
			ws.opening = false
			if err != nil {
				ws.catalogErr = err.Error()
				return
			}
			if r != nil {
				ws.addManagedRepos([]*git.Repo{r})
				remember("", r.Dir)
			}
		})
	}()
}

// removeRepository removes membership, never files, branches or remotes.
// A loaded repository keeps unsaved editor text until the existing save
// prompt has been answered, and a running Git operation cannot be removed.
func (ws *workspace) removeRepository(dir string) {
	if ws.opening {
		return
	}
	i := slices.IndexFunc(ws.apps, func(a *app) bool { return repositoryKey(a.repo.Dir) == repositoryKey(dir) })
	if i >= 0 {
		a := ws.apps[i]
		if a.busy != "" {
			return
		}
		if a.unsaved() {
			ws.activate(i)
		}
		if a.leaving(func() { ws.removeRepository(dir) }) {
			return
		}
		a.retired = true
		a.logGen++
		if a.watchCancel != nil {
			a.watchCancel()
		}
		ws.apps = slices.Delete(ws.apps, i, i+1)
		if i < ws.active {
			ws.active--
		}
		ws.active = max(0, min(ws.active, len(ws.apps)-1))
	}
	if j := repositoryIndex(prefs.Repositories, dir); j >= 0 {
		prefs.Repositories = slices.Delete(prefs.Repositories, j, j+1)
	}
	prefs.ActiveRepository = ""
	if len(ws.apps) > 0 {
		prefs.ActiveRepository = ws.apps[ws.active].repo.Dir
	}
	ws.catalogErr = ""
	prefs.save()
}

func (ws *workspace) repositoryList(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(34).PaddingX(10).Gap(6).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, loc.Repositories()).Bold().Grow(1)
			if ws.opening {
				ui.Spinner(c).Size(13, 13)
			}
			ui.Button(c, "+").Label(loc.AddRepository()).Tooltip(loc.AddRepository()).Disabled(ws.addingRepository()).Menu(ws.addRepositoryMenu)
		})
		ui.Scroll(c).Grow(1).Basis(0).Children(func() {
			for _, dir := range prefs.Repositories {
				a := (*app)(nil)
				for _, loaded := range ws.apps {
					if repositoryKey(loaded.repo.Dir) == repositoryKey(dir) {
						a = loaded
						break
					}
				}
				selected := len(ws.apps) > 0 && a == ws.apps[ws.active]
				name := filepath.Base(dir)
				row := ui.ButtonBase(c.Key(dir)).Height(30).PaddingX(10).Gap(7).AlignItems(ui.Center).
					Label(dir).Tooltip(dir).Cursor(ui.CursorPointer).Disabled(ws.opening)
				if selected {
					row.Background(t.Selection)
				} else if row.Hovered() {
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					color := t.TextMuted
					if selected {
						color = t.Warning
					}
					ui.Icon(c, icons["repo"]).Size(15, 15).TextColor(color)
					ui.Text(c, name).SingleLine()
					if a != nil {
						ui.Text(c, "("+a.status.Branch+")").SingleLine().TextColor(t.TextMuted)
					}
					ui.Box(c).Grow(1)
					if a != nil && a.busy != "" {
						ui.Spinner(c).Size(12, 12)
					}
				})
				row.OnClick(func() { ws.selectRepository(dir) }).ContextMenu(func(m *ui.Menu) {
					if m.Item(loc.RemoveRepository()).Disabled(ws.opening || a != nil && a.busy != "").Chosen() {
						ws.removeRepository(dir)
					}
				})
			}
		})
		if ws.catalogErr != "" {
			ui.Text(c, ws.catalogErr).TextColor(t.Danger).Wrap().MaxLines(4).Padding(8)
		}
	})
}

func (ws *workspace) emptyLibraryView(c *ui.Context) {
	applyDesktopTheme(c)
	ui.Column(c).Fill().Children(func() {
		titleBar(c).Children(func() { ui.Image(c, icon).Size(20, 20); ui.Text(c, "MyGit").Bold() })
		ui.Split(c, &prefs.SidebarW, func() { ws.repositoryList(c) }, func() {
			ui.Column(c).Fill().Center().Gap(14).Children(func() {
				ui.Text(c, loc.RepositoriesEmpty()).FontSize(16)
				ui.Text(c, loc.RemoveRepositoryHint()).TextColor(c.Theme().TextMuted)
				ui.PrimaryButton(c, loc.AddRepository()).Disabled(ws.addingRepository()).Menu(ws.addRepositoryMenu)
			})
		}).Grow(1)
	})
}

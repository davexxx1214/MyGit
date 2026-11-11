package main

import (
	"context"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"mygit/internal/git"
	loc "mygit/locales"
)

// Style the existing native widgets; the backend and MyGo stay unchanged.
func applyDesktopTheme(c *ui.Context) {
	t := *c.Theme()
	t.Radius = 2
	if t.Dark {
		t.Background, t.Surface = ui.RGB(29, 29, 32), ui.RGB(39, 39, 42)
		t.Border = ui.RGB(58, 58, 62)
		t.Accent, t.AccentHover, t.AccentPressed = ui.RGB(85, 91, 156), ui.RGB(102, 108, 175), ui.RGB(69, 75, 139)
		t.Selection = ui.RGB(70, 76, 133)
	}
	c.SetTheme(&t)
}

func (a *app) desktopView(c *ui.Context) {
	applyDesktopTheme(c)
	t := c.Theme()
	root := ui.Column(c).Fill().FontSize(12)
	root.Children(func() {
		titleBar(c).Background(t.Surface).Children(func() {
			ui.Image(c, icon).Size(20, 20)
			ui.Text(c, "MyGit").Bold()
			ui.Text(c, " / "+a.repo.Name()).TextColor(t.TextMuted)
			ui.Box(c).Grow(1)
			ui.Text(c, "⎇ "+a.status.Branch).TextColor(t.TextMuted)
		})
		a.desktopToolbar(c)
		a.updateBar(c)
		a.undoBar(c)
		if a.errMsg != "" || a.notice != "" {
			ui.Row(c).Padding(6, 10).Gap(8).AlignItems(ui.Center).Children(func() {
				text, color := a.notice, t.TextMuted
				if a.errMsg != "" {
					text, color = a.errMsg, t.Danger
				}
				ui.Text(c, text).Wrap().MaxLines(3).TextColor(color).Grow(1)
				if ui.Button(c, loc.Close()).Clicked() {
					a.errMsg, a.notice = "", ""
				}
			})
		}
		a.operationBar(c)
		content := func() {
			ui.SplitVertical(c.Key("desktop-content"), &a.logH, func() {
				if a.desktopLog {
					a.logView(c)
				} else {
					a.leftPanel(c)
				}
			}, func() { a.detailView(c) }).Fill()
		}
		if prefs.SidebarHidden {
			ui.Column(c).Grow(1).Basis(0).Children(content)
		} else {
			ui.Split(c.Key("desktop-main"), &a.sidebarW, func() {
				h := &prefs.RepositoriesH
				if *h <= 0 {
					*h = 320
				}
				ui.SplitVertical(c.Key("desktop-navigation"), h, func() { a.ws.repositoryList(c) }, func() { a.branchesPanel(c) }).Fill()
			}, content).Grow(1)
		}
		if root.FileDragOver() {
			dropHint(c)
		}
	})
	if paths := root.DroppedFiles(); len(paths) > 0 {
		a.addDroppedRepositories(paths)
	}
	// All existing operation dialogs share the active repository's state.
	a.rebaseView(c)
	a.rewordView(c)
	a.pushView(c)
	a.promptView(c)
	a.worktreeView(c)
	a.remotesView(c)
	a.settingsView(c)
	a.confirmView(c)
	a.cred.view(c)
	a.paletteView(c)
	a.composerView(c)
	a.pullsView(c)
	a.submitView(c)
	c.OnShortcut(ui.Cmd, ui.KeyR, a.refresh)
	c.OnShortcut(ui.Cmd, ui.KeyB, a.toggleSidebar)
	c.OnShortcut(ui.Cmd, ui.KeyP, a.openFiles)
	c.OnShortcut(ui.Cmd, ui.KeyK, a.openCommands)
	c.OnShortcut(ui.Cmd, ui.KeyL, func() { a.setDesktopLog(true) })
	c.OnShortcut(ui.Cmd, ui.KeyEnter, func() { a.setDesktopLog(false); a.commit() })
	c.OnShortcut(0, ui.KeyF7, func() { a.stepChange(false) })
	c.OnShortcut(ui.Shift, ui.KeyF7, func() { a.stepChange(true) })
	if !macOS {
		c.OnShortcut(ui.Cmd, ui.KeyComma, func() { a.settings.open = true })
	}
}

func (a *app) setDesktopLog(log bool) {
	a.desktopLog, prefs.DesktopLog = log, log
	prefs.save()
}

func desktopAction(c *ui.Context, key, icon, text, label string, selected, disabled bool) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Width(76).Height(58).Gap(4).Center().Radius(2).
		Label(label).Tooltip(label).Cursor(ui.CursorPointer).Disabled(disabled)
	if selected {
		b.Background(t.Selection.Alpha(0.6)).BorderWidth(0, 0, 2, 0).BorderColor(t.Accent)
	} else if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Column(c).Center().Gap(4).Children(func() {
			ui.Icon(c, icons[icon]).Size(21, 21)
			ui.Text(c, text).FontSize(12)
		})
	})
	return b
}

func (a *app) desktopToolbar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(68).Gap(4).PaddingX(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		idle := a.busy == ""
		desktopAction(c, "pull", "download", loc.Pull()+" ▾", loc.Pull(), false, !idle || a.status.Upstream == "").Menu(func(m *ui.Menu) {
			pull := func(mode git.PullMode) {
				a.runPull(loc.OpPull(), func(ctx context.Context) error { return a.repo.Pull(ctx, mode) })
			}
			if m.Item(loc.PullFF()).Chosen() {
				pull(git.PullFastForward)
			}
			if m.Item(loc.PullRebase()).Chosen() {
				pull(git.PullRebase)
			}
			if m.Item(loc.PullMerge()).Chosen() {
				pull(git.PullMerge)
			}
		})
		desktopAction(c, "commit", "check", loc.Commit(), loc.CommitView(), !a.desktopLog, false).OnClick(func() { a.setDesktopLog(false) })
		desktopAction(c, "discard", "undo", loc.Discard(), loc.Discard(), false, !idle || len(a.unstaged) == 0).Menu(func(m *ui.Menu) {
			ch := a.detail.change
			if ch != nil && !a.detail.staged && !ch.Conflict {
				if m.Item(loc.DiscardChanges()).Chosen() {
					a.discard(*ch)
				}
				m.Separator()
			}
			if m.Item(loc.DiscardAll()).Chosen() {
				a.discardAll()
			}
		})
		desktopAction(c, "log", "repo", "Log", loc.LogView(), a.desktopLog, false).OnClick(func() { a.setDesktopLog(true) })
		ui.Box(c).Width(1).Height(42).Background(t.Border).Margin(0, 8)
		if ui.Button(c, loc.Fetch()).Disabled(!idle).Clicked() {
			a.run(loc.OpFetch(), a.repo.Fetch, nil)
		}
		if ui.Button(c, loc.PushMenu()).Disabled(!idle || a.status.Branch == "").Clicked() {
			a.openPush()
		}
		if ui.Button(c, loc.Refresh()).Clicked() {
			a.refresh()
		}
		ui.Box(c).Grow(1)
		if a.busy != "" {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, a.busy+"…").TextColor(t.TextMuted)
		}
		ui.Text(c, a.status.Upstream).TextColor(t.TextMuted).SingleLine()
		a.commentsButton(c)
		ui.Button(c, "⋯").Label(loc.Repository()).Menu(a.repoMenu)
	})
}

func (a *app) branchesPanel(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(34).PaddingX(10).AlignItems(ui.Center).Children(func() { ui.Text(c, loc.Branches()).Bold().Grow(1) })
		ui.TextInput(c, &a.branchFilter).Placeholder(loc.BranchSearch()).Label(loc.BranchSearch()).Margin(4, 8)
		ui.Scroll(c).Grow(1).Basis(0).Children(func() {
			q := strings.ToLower(strings.TrimSpace(a.branchFilter))
			group := func(kind, title string, names []string, remote bool) {
				filtered := []string{}
				for _, name := range names {
					if q == "" || strings.Contains(strings.ToLower(name), q) {
						filtered = append(filtered, name)
					}
				}
				if !sectionHeader(c, kind, title, len(filtered), q != "") {
					return
				}
				for _, name := range filtered {
					shown := name
					if remote {
						_, shown, _ = strings.Cut(name, "/")
					}
					row := branchRowElement(c.Key(kind+name), shown, currentNote(!remote && name == a.status.Branch), !remote && name == a.status.Branch).
						Label(name).Tooltip(name)
					row.OnClick(func() { a.setDesktopLog(true); a.setScope(git.Scope{Ref: name}) }).ContextMenu(func(m *ui.Menu) {
						if remote {
							a.remoteItems(m, name)
						} else {
							a.branchItems(m, name)
						}
					})
				}
			}
			group("desktop-local", loc.BranchesLocal(), a.branches, false)
			var remotes []string
			for _, name := range a.remotes {
				remote, _, ok := strings.Cut(name, "/")
				if ok && !slices.Contains(remotes, remote) {
					remotes = append(remotes, remote)
				}
			}
			slices.Sort(remotes)
			for _, remote := range remotes {
				var names []string
				for _, name := range a.remotes {
					if strings.HasPrefix(name, remote+"/") {
						names = append(names, name)
					}
				}
				group("desktop-remote-"+remote, remote, names, true)
			}
			if len(a.branches)+len(a.remotes) == 0 {
				ui.Text(c, loc.BranchesNone()).TextColor(t.TextMuted).Padding(10)
			}
		})
	})
}

func (a *app) addDroppedRepositories(paths []string) {
	if a.ws.opening {
		return
	}
	a.ws.opening = true
	go func() {
		var repos []*git.Repo
		var failure error
		for _, p := range paths {
			r, err := git.Open(dropTarget(p))
			if err != nil {
				failure = err
			} else {
				repos = append(repos, r)
			}
		}
		a.win.Update(func() {
			a.ws.opening = false
			a.ws.addManagedRepos(repos)
			if failure != nil {
				a.ws.catalogErr = failure.Error()
			}
		})
	}()
}

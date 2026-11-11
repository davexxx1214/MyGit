package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygit/internal/git"
	loc "mygit/locales"
)

// clonePlan is the clone dialog: a URL, the folder to clone into, and the
// clone's progress once it runs.
type clonePlan struct {
	open           bool
	url, parent    string
	name           string // the folder's name, from the URL until edited
	named          bool
	running        bool
	progress, fail string
	cancel         context.CancelFunc
}

// defaultParent is where clones go: ~/Projects when there is one.
func defaultParent() string {
	home, _ := os.UserHomeDir()
	for _, d := range []string{"Projects", "projects", "Code", "code", "src"} {
		if fi, err := os.Stat(filepath.Join(home, d)); err == nil && fi.IsDir() {
			return filepath.Join(home, d)
		}
	}
	return home
}

func (w *welcome) cloneView(c *ui.Context) {
	w.clone.view(c, w.win, w.update, w.open)
}

// view shares the clone form and runner between the welcome window and
// the repository library; successful clones are handed back to their host.
func (p *clonePlan) view(c *ui.Context, win *mygo.Window, update func(func()), opened func(string)) {
	if !p.open {
		return
	}
	t := c.Theme()
	if ui.Modal(c, &p.open, func() {
		ui.Column(c).Gap(10).Width(dialogWidth(c, 460)).Children(func() {
			ui.Text(c, loc.CloneTitle()).Bold().FontSize(15)
			if ui.TextInput(c, &p.url).Placeholder(loc.CloneURLPlaceholder()).Label(loc.CloneURL()).Disabled(p.running).AutoFocus().Changed() && !p.named {
				p.name = git.RepoName(strings.TrimSpace(p.url))
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, homeRelative(p.parent)).TextColor(t.TextMuted).SingleLine().Grow(1).Basis(0).Tooltip(p.parent)
				if ui.Button(c, loc.Change()).Disabled(p.running).Clicked() {
					go p.pickParent(win, update)
				}
			})
			if ui.TextInput(c, &p.name).Label(loc.FolderName()).Placeholder(loc.FolderName()).Disabled(p.running).Changed() {
				p.named = true
			}
			switch {
			case p.running:
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(14, 14)
					ui.Text(c, p.progress).TextColor(t.TextMuted).SingleLine().Grow(1).Basis(0)
				})
			case p.fail != "":
				ui.Text(c, p.fail).TextColor(t.Danger).Wrap()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, loc.Cancel()).Clicked() {
					p.stop()
				}
				ok := strings.TrimSpace(p.url) != "" && strings.TrimSpace(p.name) != "" && !p.running
				if ui.PrimaryButton(c, loc.Clone()).Disabled(!ok).Clicked() {
					p.start(update, opened)
				}
			})
		})
	}).Dismissed() {
		p.stop()
	}
}

func (p *clonePlan) stop() {
	if p.cancel != nil {
		p.cancel()
	}
	*p = clonePlan{}
}

func (p *clonePlan) pickParent(win *mygo.Window, update func(func())) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: loc.CloneInto(), Directory: true, CreateDirectories: true})
	if err == nil && len(paths) > 0 {
		update(func() {
			if p.open && !p.running {
				p.parent = paths[0]
			}
		})
	}
}

// start clones on a goroutine, showing git's progress, and opens the
// clone once done.
func (p *clonePlan) start(update func(func()), opened func(string)) {
	if p.running {
		return
	}
	url, parent, name := strings.TrimSpace(p.url), p.parent, strings.TrimSpace(p.name)
	ctx, cancel := context.WithCancel(context.Background())
	p.running, p.fail, p.progress, p.cancel = true, "", loc.Connecting(), cancel
	go func() {
		err := git.Clone(ctx, parent, url, name, func(line string) {
			update(func() {
				if ctx.Err() == nil {
					p.progress = line
				}
			})
		})
		update(func() {
			defer cancel()
			if ctx.Err() != nil {
				return // canceled
			}
			p.running = false
			if err != nil {
				p.fail = err.Error()
				return
			}
			p.open = false
			opened(filepath.Join(parent, name))
		})
	}()
}

// initRepo asks for a folder, makes a repository in it and opens it. The
// dialog waits: call it on a goroutine.
func (w *welcome) initRepo() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: w.win, Title: loc.InitDialog(), Directory: true, CreateDirectories: true})
	if err != nil || len(paths) == 0 {
		return
	}
	dir := paths[0]
	err = git.Init(context.Background(), dir)
	w.win.Update(func() {
		if err != nil {
			w.errMsg = err.Error()
			return
		}
		w.open(dir)
	})
}

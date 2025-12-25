package pstree

import (
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kevwargo/go-pst/internal/pager"
	"github.com/kevwargo/go-pst/internal/procwatch"
)

func runTUI(tree *Tree, pg *pager.Pager) error {
	watcher, err := procwatch.Watch()
	if err != nil {
		return err
	}

	var opts []tea.ProgramOption
	if tree.cfg.Fullscreen {
		opts = append(opts, tea.WithAltScreen())
	}

	t := tui{
		tree:       tree,
		pager:      pg,
		watcher:    watcher,
		fullscreen: tree.cfg.Fullscreen,
	}

	lf, err := t.openLog()
	if err != nil {
		return err
	}
	defer lf.Close()

	_, err = tea.NewProgram(&t, opts...).Run()

	return err
}

type tui struct {
	tree       *Tree
	pager      *pager.Pager
	watcher    procwatch.Watcher
	fullscreen bool
	quitting   bool
}

func (t *tui) Init() tea.Cmd {
	return t.recvMsg
}

func (t *tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		cmd = t.handleKey(msg)
	case tea.WindowSizeMsg:
		t.handleWinSize(msg)
	case procMsg:
		cmd = t.handleProcMsg(msg)
	}

	return t, tea.Sequence(cmd, t.recvMsg)
}

func (t *tui) View() string {
	if t.quitting {
		return ""
	}

	return t.pager.String()
}

type procMsg struct {
	event any
	err   error
}

func (t *tui) recvMsg() tea.Msg {
	var msg procMsg
	msg.event, msg.err = t.watcher.Recv()

	return msg
}

func (t *tui) handleProcMsg(msg procMsg) tea.Cmd {
	if msg.event == nil {
		if t.quitting {
			return nil
		}

		t.quitting = true
		t.pager.SetMaxHeight(0)
		cmd := tea.Println(t.pager.String())

		if msg.err != nil {
			cmd = tea.Sequence(cmd, tea.Printf("procwatcher error: %s", msg.err.Error()))
		}

		return tea.Sequence(cmd, tea.Quit)
	}

	var (
		changed bool
		err     error
	)

	switch ev := msg.event.(type) {
	case procwatch.EventForkProc:
		changed, err = t.tree.insertProcess(ev.PID, ev.ParentPID)
	case procwatch.EventForkThread:
		changed, err = t.tree.insertThread(ev.TID, ev.PID)
	case procwatch.EventExec:
		changed, err = t.tree.reloadProcess(ev.PID)
	case procwatch.EventExitProc:
		changed, err = t.tree.removeProcess(ev.PID, ev.ParentPID, ev.ExitCode, ev.ExitSignal)
	case procwatch.EventExitThread:
		changed, err = t.tree.removeThread(ev.TID, ev.PID)
	}

	if err != nil {
		return tea.Printf("handling %T: %v", msg.event, err)
	}

	if changed {
		t.tree.render(t.pager)
	}

	return nil
}

func (t *tui) handleKey(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd

	switch k := msg.String(); k {
	case "q", "ctrl+c":
		cmd = t.closeWatcher
	case "d":
		t.toggleShowDead()
	case "t":
		t.toggleShowThreads()
	case "f":
		cmd = t.toggleFullscreen()
	case "up":
		t.pager.Up()
	case "down":
		t.pager.Down()
	}

	return cmd
}

func (t *tui) closeWatcher() tea.Msg {
	t.watcher.Close()

	return struct{}{}
}

func (t *tui) handleWinSize(msg tea.WindowSizeMsg) {
	t.pager.SetMaxWidth(msg.Width)
	t.pager.SetMaxHeight(msg.Height)
}

func (t *tui) toggleFullscreen() tea.Cmd {
	t.fullscreen = !t.fullscreen

	if t.fullscreen {
		return tea.EnterAltScreen
	}

	return tea.ExitAltScreen
}

func (t *tui) toggleShowDead() {
	// TODO: FIXME: sometimes toggling this hides the process that has dead children
	// but scrolling shows this process again.
	t.tree.cfg.ShowDead = !t.tree.cfg.ShowDead
	t.tree.render(t.pager)
}

func (t *tui) toggleShowThreads() {
	t.tree.cfg.ShowThreads = !t.tree.cfg.ShowThreads

	if t.tree.cfg.ShowThreads {
		for _, p := range t.tree.pMap {
			p.loadThreads(t.tree.cfg)
		}
	} else {
		for _, p := range t.tree.pMap {
			p.threads = nil
		}
	}

	t.tree.render(t.pager)
}

func (t *tui) openLog() (*os.File, error) {
	lf, err := os.OpenFile("pst.log", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}

	log.SetOutput(lf)

	return lf, nil
}

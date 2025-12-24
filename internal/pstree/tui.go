package pstree

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kevwargo/go-pst/internal/procwatch"
)

func runTUI(tree *Tree) error {
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
		watcher:    watcher,
		fullscreen: tree.cfg.Fullscreen,
	}

	_, err = tea.NewProgram(&t, opts...).Run()

	return err
}

type tui struct {
	tree       *Tree
	watcher    procwatch.Watcher
	fullscreen bool
}

func (t *tui) Init() tea.Cmd {
	return t.recvMsg
}

func (t *tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		cmd = t.handleKey(msg)
	case procMsg:
		cmd = t.handleProcMsg(msg)
	}

	return t, tea.Sequence(cmd, t.recvMsg)
}

func (t *tui) View() string {
	var buf strings.Builder
	t.tree.dump(&buf)

	return buf.String()
}

func (t *tui) quit() tea.Msg {
	t.watcher.Close()

	return tea.QuitMsg{}
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
	if msg.err != nil {
		return tea.Sequence(tea.Printf("procwatcher error: %s", msg.err.Error()), tea.Quit)
	}

	if msg.event == nil {
		return tea.Sequence(tea.Println("procwatcher finished"), tea.Quit)
	}

	switch ev := msg.event.(type) {
	case procwatch.EventForkProc:
		t.tree.insertProcess(ev.PID, ev.ParentPID)
	// case procwatch.EventForkThread:
	// 	cmd = tea.Printf("thread %d -> %d", ev.PID, ev.TID)
	case procwatch.EventExec:
		t.tree.reloadProcess(ev.PID)
	case procwatch.EventExitProc:
		t.tree.removeProcess(ev.PID, ev.ParentPID, ev.ExitCode, ev.ExitSignal)
		// case procwatch.EventExitThread:
		// 	cmd = tea.Printf("exit-thread %d (process:%d)", ev.TID, ev.PID)
	}

	return nil
}

func (t *tui) handleKey(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd

	switch k := msg.String(); k {
	case "q", "ctrl+c":
		cmd = t.quit
	case "d":
		t.tree.toggleShowDead()
	case "f":
		cmd = t.toggleFullscreen()
	}

	return cmd
}

func (t *tui) toggleFullscreen() tea.Cmd {
	t.fullscreen = !t.fullscreen

	if t.fullscreen {
		return tea.EnterAltScreen
	}

	return tea.ExitAltScreen
}

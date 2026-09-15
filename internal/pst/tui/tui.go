package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/kevwargo/go-pst/internal/benchmark"
	"github.com/kevwargo/go-pst/internal/procwatch"
	"github.com/kevwargo/go-pst/internal/pst/tree"
	"github.com/kevwargo/go-pst/internal/pst/tui/keymap"
)

type Config struct {
	Fullscreen bool
}

func Run(cfg *Config, pst *tree.Tree) error {
	watcher, err := procwatch.Watch()
	if err != nil {
		return err
	}

	t := tui{
		cfg:     cfg,
		pst:     pst,
		watcher: watcher,
	}

	p := tea.NewProgram(&t)
	_, err = p.Run()

	return errors.Join(err, t.err)
}

type tui struct {
	cfg     *Config
	pst     *tree.Tree
	watcher procwatch.Watcher
	keymap  *keymap.Keymap

	width    int
	height   int
	showHelp bool
	err      error
	quitting bool
}

func (t *tui) Init() tea.Cmd {
	t.keymap = keymap.New().
		AddCmd("q", "Close program", t.closeWatcher).
		AddCmd("w", "Force adjust to window size", t.adjustWinSize).
		AddCmd("r", "Reload whole tree", t.reload).
		AddFunc("?", "Toggle help", func() { t.showHelp = !t.showHelp }).
		AddFunc("d", "Toggle show-dead", t.pst.ToggleShowDead).
		AddFunc("D", "Cleanup dead", t.pst.CleanupDead).
		AddFunc("t", "Toggle threads", t.pst.ToggleThreads).
		AddFunc("f", "Toggle fullscreen", func() { t.cfg.Fullscreen = !t.cfg.Fullscreen }).
		AddFunc("up", "Up 1 line", func() { t.pst.GetPager().Up() }).
		AddFunc("down", "Down 1 line", func() { t.pst.GetPager().Down() }).
		AddFunc("pgup", "Up 1 page", func() { t.pst.GetPager().PageUp() }).
		AddFunc("pgdown", "Down 1 page", func() { t.pst.GetPager().PageDown() }).
		AddFunc("left", "Left 5 chars", func() { t.pst.GetPager().Left(5) }).
		AddFunc("right", "Right 5 chars", func() { t.pst.GetPager().Right(5) })

	return t.recvMsg
}

func (t *tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer benchmark.Record("tui.Update", time.Now())

	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		cmd = t.keymap.HandleKey(msg)
	case tea.WindowSizeMsg:
		t.handleWinSize(msg)
	case procwatch.Message:
		cmd = t.handleProcMsg(msg)
	case reloadSuccessMsg:
		// nothing, just some non-nil message to trigger Update()->View()->render cycle
	}

	return t, cmd
}

func (t *tui) View() (v tea.View) {
	v.SetContent(t.pst.View())
	if v.Content != "" && !strings.HasSuffix(v.Content, "\n") {
		v.SetContent(v.Content + "\n")
	}

	if t.showHelp {
		v.SetContent(v.Content + t.keymap.Help())
	}

	v.Cursor = tea.NewCursor(0, 0)
	v.Cursor.Blink = false

	v.AltScreen = t.cfg.Fullscreen

	return v
}

func (t *tui) recvMsg() tea.Msg {
	return t.watcher.Recv(2 * time.Second)
}

func reprMsg(msg tea.Msg) string {
	s := fmt.Sprintf("%T(%+v)", msg, msg)
	if len(s) > 100 {
		s = s[:100]
	}

	return s
}

func (t *tui) handleProcMsg(msg procwatch.Message) tea.Cmd {
	if msg.EOF || msg.Err != nil {
		return t.handleQuitMsg(msg.Err)
	}

	if msg.Timeout {
		return tea.Sequence(t.reload, t.recvMsg)
	}

	switch ev := msg.Event.(type) {
	case procwatch.EventForkProc:
		t.pst.HandleNewProcess(ev)
	case procwatch.EventForkThread:
		t.pst.HandleNewThread(ev)
	case procwatch.EventExec:
		t.pst.HandleExec(ev)
	case procwatch.EventComm:
		t.pst.HandleComm(ev)
	case procwatch.EventExitProc:
		t.pst.HandleProcessExit(ev)
	case procwatch.EventExitThread:
		t.pst.HandleThreadExit(ev)
	}

	return t.recvMsg
}

func (t *tui) handleQuitMsg(procWatchErr error) tea.Cmd {
	if t.quitting {
		return nil
	}

	t.quitting = true
	t.pst.GetPager().SetMaxHeight(0)

	if procWatchErr != nil {
		t.err = errors.Join(t.err, fmt.Errorf("procwatcher error: %w", procWatchErr))
	}

	return tea.Quit
}

func (t *tui) reload() tea.Msg {
	if err := t.pst.Reload(); err != nil {
		t.err = err

		return t.closeWatcher()
	}

	return reloadSuccessMsg{}
}

type reloadSuccessMsg struct{}

func (t *tui) adjustWinSize() tea.Msg {
	return tea.WindowSizeMsg{
		Width:  t.width,
		Height: t.height,
	}
}

func (t *tui) closeWatcher() tea.Msg {
	t.watcher.Close()

	return nil
}

func (t *tui) handleWinSize(msg tea.WindowSizeMsg) {
	t.width = msg.Width
	t.height = msg.Height
	t.pst.GetPager().SetMaxWidth(msg.Width - 1)
	t.pst.GetPager().SetMaxHeight(msg.Height - 1)
}

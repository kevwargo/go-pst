package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/kevwargo/go-pst/internal/benchmark"
	"github.com/kevwargo/go-pst/internal/procwatch"
	"github.com/kevwargo/go-pst/internal/pst/tree"
	"github.com/kevwargo/go-pst/internal/pst/tui/keymap"
)

type Config struct {
	Fullscreen bool
	DebugRecv  bool
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

	width       int
	height      int
	showHelp    bool
	lastKey     tea.KeyMsg
	showLastKey bool

	recvCount  int
	timerCount int

	err      error
	quitting bool
}

func (t *tui) Init() tea.Cmd {
	t.keymap = keymap.New().
		AddFunc("up", "Up 1 line", t.pagerUp).
		AddFunc("down", "Down 1 line", t.pagerDown).
		AddFunc("pgup", "Up 1 page", t.pagerPgup).
		AddFunc("pgdown", "Down 1 page", t.pagerPgdown).
		AddFunc("left", "Left 5 chars", t.pagerLeft(5)).
		AddFunc("right", "Right 5 chars", t.pagerRight(5)).
		AddFunc(",", "Left 1 char", t.pagerLeft(1)).
		AddFunc(".", "Right 1 char", t.pagerRight(1)).
		AddFunc("home", "Scroll home", t.pagerHome).
		AddFunc("end", "Scroll to end", t.pagerEnd).
		NewGroup().
		AddFunc("?", "Toggle help", t.toggleHelp, "h").
		AddFunc("d", "Toggle show-dead", t.pst.ToggleShowDead).
		AddFunc("t", "Toggle threads", t.pst.ToggleThreads).
		AddFunc("f", "Toggle fullscreen", t.toggleFullscreen).
		AddFunc("K", "Toggle last key", t.toggleLastKey).
		NewGroup().
		AddCmd("w", "Force adjust to window size", t.adjustWinSize).
		AddCmd("r", "Refresh tree", t.refreshManual).
		AddFunc("D", "Cleanup dead", t.pst.CleanupDead).
		AddCmd("ctrl+c", "Close program", t.closeWatcher, "q", "esc")

	return tea.Batch(t.recvMsg, tickRefresh())
}

func (t *tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer benchmark.Record("tui.Update", time.Now())

	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		t.lastKey = msg
		cmd = t.keymap.HandleKey(msg)
	case tea.WindowSizeMsg:
		t.handleWinSize(msg)
	case procwatch.Message:
		cmd = t.handleProcMsg(msg)
	case refreshMsg:
		cmd = t.handleRefresh(msg)
	}

	return t, cmd
}

func (t *tui) View() (v tea.View) {
	buf := bytes.NewBufferString(t.pst.View())

	if t.showHelp {
		fmt.Fprint(buf, "\n", t.keymap.Help())
		if t.showLastKey && t.lastKey != nil {
			fmt.Fprintf(buf, "\nLast key: %q", t.lastKey.String())
			buf.WriteString(ansi.Style{}.ForegroundColor(ansi.Green).String())
			json.NewEncoder(buf).Encode(t.lastKey.Key())
			buf.WriteString(ansi.ResetStyle)
		}
	}
	buf.WriteByte('\n')

	if t.cfg.DebugRecv {
		fmt.Fprintf(buf, "recv:%d timer:%d\n", t.recvCount, t.timerCount)
	}

	v.SetContent(buf.String())

	v.Cursor = &tea.Cursor{Shape: tea.CursorBlock}
	v.AltScreen = t.cfg.Fullscreen

	return v
}

func (t *tui) recvMsg() tea.Msg {
	msg := t.watcher.Recv()
	t.recvCount++

	return msg
}

func (t *tui) handleProcMsg(msg procwatch.Message) tea.Cmd {
	if msg.EOF || msg.Err != nil {
		return t.handleQuitMsg(msg.Err)
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

func (t *tui) toggleHelp() {
	t.showHelp = !t.showHelp
}

func (t *tui) toggleFullscreen() {
	t.cfg.Fullscreen = !t.cfg.Fullscreen
}

func (t *tui) toggleLastKey() {
	t.showLastKey = !t.showLastKey
}

func (t *tui) pagerUp() {
	t.pst.GetPager().Up()
}

func (t *tui) pagerDown() {
	t.pst.GetPager().Down()
}

func (t *tui) pagerPgup() {
	t.pst.GetPager().PageUp()
}

func (t *tui) pagerPgdown() {
	t.pst.GetPager().PageDown()
}

func (t *tui) pagerLeft(delta uint) func() {
	return func() { t.pst.GetPager().Left(delta) }
}

func (t *tui) pagerRight(delta uint) func() {
	return func() { t.pst.GetPager().Right(delta) }
}

func (t *tui) pagerHome() {
	t.pst.GetPager().FullLeft()
}

func (t *tui) pagerEnd() {
	t.pst.GetPager().FullRight()
}

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

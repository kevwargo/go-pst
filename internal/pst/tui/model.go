package tui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kevwargo/go-pst/internal/benchmark"
	"github.com/kevwargo/go-pst/internal/procwatch"
	"github.com/kevwargo/go-pst/internal/pst/tree"
)

type Config struct {
	Fullscreen bool
}

func Run(cfg *Config, pst *tree.Tree) error {
	watcher, err := procwatch.Watch()
	if err != nil {
		return err
	}

	var opts []tea.ProgramOption
	if cfg.Fullscreen {
		opts = append(opts, tea.WithAltScreen())
	}

	t := tui{
		cfg:     cfg,
		pst:     pst,
		watcher: watcher,
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
	cfg     *Config
	pst     *tree.Tree
	watcher procwatch.Watcher

	width    int
	height   int
	quitting bool
}

func (t *tui) Init() tea.Cmd {
	return t.recvMsg
}

func (t *tui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer benchmark.Record("tui.Update", time.Now())

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

	return t.pst.View() + "\n"
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
		t.pst.GetPager().SetMaxHeight(0)
		cmd := tea.Println(t.pst.View())

		if msg.err != nil {
			cmd = tea.Sequence(cmd, tea.Printf("procwatcher error: %s", msg.err.Error()))
		}

		return tea.Sequence(cmd, tea.Quit)
	}

	switch ev := msg.event.(type) {
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

	return nil
}

func (t *tui) handleKey(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd

	switch k := msg.String(); k {
	case "q", "ctrl+c":
		cmd = t.closeWatcher
	case "d":
		t.pst.ToggleShowDead()
	case "D":
		t.pst.CleanupDead()
	case "t":
		t.pst.ToggleThreads()
	case "f":
		cmd = t.toggleFullscreen()
	case "r":
		cmd = t.forceRefresh
	case "up":
		t.pst.GetPager().Up()
	case "down":
		t.pst.GetPager().Down()
	case "pgup":
		t.pst.GetPager().PageUp()
	case "pgdown":
		t.pst.GetPager().PageDown()
	case "left":
		t.pst.GetPager().Left()
	case "right":
		t.pst.GetPager().Right()
	}

	return cmd
}

func (t *tui) forceRefresh() tea.Msg {
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

func (t *tui) toggleFullscreen() tea.Cmd {
	t.cfg.Fullscreen = !t.cfg.Fullscreen

	if t.cfg.Fullscreen {
		return tea.EnterAltScreen
	}

	return tea.ExitAltScreen
}

func (t *tui) openLog() (*os.File, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}

	lf, err := os.OpenFile(filepath.Join(cacheDir, "pst.log"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}

	log.SetOutput(lf)

	return lf, nil
}

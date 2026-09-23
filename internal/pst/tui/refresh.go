package tui

import (
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

type refreshMsg int

const (
	refreshTimer refreshMsg = iota
	refreshOndemand
)

func (m refreshMsg) String() string {
	switch m {
	case refreshTimer:
		return "Refresh on timer"
	case refreshOndemand:
		return "Refresh on demand"
	default:
		return "Unrecognized refreshMsg"
	}
}

func (t *tui) handleRefresh(msg refreshMsg) (cmd tea.Cmd) {
	err := t.pst.Reload()

	if err != nil {
		t.err = errors.Join(t.err, fmt.Errorf("tree refresh: %w", err))
		cmd = t.closeWatcher
	} else if msg == refreshTimer {
		cmd = tickRefresh()
	}

	return cmd
}

func (t *tui) refreshManual() tea.Msg {
	return refreshOndemand
}

func tickRefresh() tea.Cmd {
	return tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
		return refreshTimer
	})
}

package keymap

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	bubblekey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/kevwargo/go-pst/internal/benchmark"
)

type Keymap struct {
	bindings []bubblekey.Binding
	m        map[string]*action
}

type action struct {
	cmd tea.Cmd
	fn  func()
}

func New() *Keymap {
	return &Keymap{}
}

func (km *Keymap) AddCmd(key, description string, cmd tea.Cmd, additionalKeys ...string) *Keymap {
	if cmd != nil {
		km.addAction(key, description, &action{cmd: cmd}, additionalKeys...)
	}

	return km
}

func (km *Keymap) AddFunc(key, description string, fn func(), additionalKeys ...string) *Keymap {
	if fn != nil {
		km.addAction(key, description, &action{fn: fn}, additionalKeys...)
	}

	return km
}

func (km *Keymap) HandleKey(key tea.KeyMsg) tea.Cmd {
	if act := km.m[key.String()]; act != nil {
		switch {
		case act.cmd != nil:
			return act.cmd
		case act.fn != nil:
			act.fn()
		}
	}

	return nil
}

func (km *Keymap) Help(maxWidth int) string {
	defer benchmark.Record("keymap", time.Now())

	last := km.renderHelpColumns(1)

	for ncol := 2; ncol <= len(km.bindings); ncol++ {
		columns := km.renderHelpColumns(ncol)
		if lipgloss.Width(columns) > maxWidth {
			break
		}

		last = columns
	}

	return last
}

func (km *Keymap) renderHelpColumns(ncol int) string {
	var columns []string
	n := math.Floor(float64(len(km.bindings)) / float64(ncol))

	for chunk := range slices.Chunk(km.bindings, int(n)) {
		var keys, descs []string

		for _, b := range chunk {
			var bk []string
			for _, k := range b.Keys() {
				bk = append(bk, styleKey.Render(k))
			}
			keys = append(keys, strings.Join(bk, "|"))
			descs = append(descs, styleDescription.Render(b.Help().Desc))
		}

		columns = append(columns,
			lipgloss.JoinHorizontal(lipgloss.Top,
				strings.Join(keys, "\n"),
				" ",
				strings.Join(descs, "\n"),
			),
			"  ",
		)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, columns...)
}

func (km *Keymap) addAction(key, description string, act *action, additionalKeys ...string) {
	defer benchmark.Record("keymap", time.Now())

	if km.m == nil {
		km.m = make(map[string]*action)
	}

	keys := append([]string{key}, additionalKeys...)
	for _, k := range keys {
		if _, ok := km.m[k]; ok {
			panic(fmt.Sprintf("duplicate key %q", k))
		}

		km.m[k] = act
	}

	km.bindings = append(km.bindings, bubblekey.NewBinding(
		bubblekey.WithKeys(keys...),
		bubblekey.WithHelp("" /*unused*/, description),
	))
}

var (
	styleKey         = lipgloss.NewStyle().Foreground(lipgloss.Color("#aa55ff")).Bold(true)
	styleDescription = lipgloss.NewStyle().Foreground(lipgloss.Color("#0090ff")).Italic(true)
)

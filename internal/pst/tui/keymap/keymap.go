package keymap

import (
	"strings"

	"charm.land/bubbles/v2/help"
	bubblekey "charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type Keymap struct {
	layout   [][]bubblekey.Binding
	bindings map[string]*action
	newGroup bool
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

func (km *Keymap) NewGroup() *Keymap {
	km.newGroup = true
	return km
}

func (km *Keymap) HandleKey(key tea.KeyMsg) tea.Cmd {
	if act := km.bindings[key.String()]; act != nil {
		switch {
		case act.cmd != nil:
			return act.cmd
		case act.fn != nil:
			act.fn()
		}
	}

	return nil
}

func (km *Keymap) addAction(key, description string, act *action, additionalKeys ...string) {
	if km.bindings == nil {
		km.bindings = make(map[string]*action)
	}

	keys := append([]string{key}, additionalKeys...)
	for _, k := range keys {
		km.bindings[k] = act
	}

	binding := bubblekey.NewBinding(
		bubblekey.WithKeys(keys...),
		bubblekey.WithHelp(strings.Join(keys, " | "), description),
	)

	if km.newGroup || len(km.layout) == 0 {
		km.layout = append(km.layout, []bubblekey.Binding{binding})
		km.newGroup = false
	} else {
		km.layout[len(km.layout)-1] = append(km.layout[len(km.layout)-1], binding)
	}
}

func (km *Keymap) Help() string {
	h := help.New()
	h.Styles.FullKey = h.Styles.FullKey.Foreground(ansi.BrightGreen).Bold(true)
	h.Styles.FullDesc = h.Styles.FullDesc.Foreground(ansi.BrightBlue).Bold(true)

	return h.FullHelpView(km.layout)
}

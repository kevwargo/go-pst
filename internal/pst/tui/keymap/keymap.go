package keymap

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type Keymap struct {
	bindings []binding
}

type binding struct {
	keys        []string
	description string
	cmd         tea.Cmd
	fn          func()
}

func New() *Keymap {
	return &Keymap{}
}

func (km *Keymap) AddCmd(key, description string, cmd tea.Cmd, additionalKeys ...string) *Keymap {
	if cmd != nil {
		km.bindings = append(km.bindings, binding{
			keys:        append([]string{key}, additionalKeys...),
			description: description,
			cmd:         cmd,
		})
	}

	return km
}

func (km *Keymap) AddFunc(key, description string, fn func(), additionalKeys ...string) *Keymap {
	if fn != nil {
		km.bindings = append(km.bindings, binding{
			keys:        append([]string{key}, additionalKeys...),
			description: description,
			fn:          fn,
		})
	}

	return km
}

func (km *Keymap) HandleKey(key tea.KeyMsg) tea.Cmd {
	for _, b := range km.bindings {
		if slices.Contains(b.keys, key.String()) {
			if b.cmd != nil {
				return b.cmd
			}

			if b.fn != nil {
				b.fn()
			}

			break
		}
	}

	return nil
}

func (km *Keymap) Help() string {
	bindings := make([]key.Binding, 0, len(km.bindings))
	for _, b := range km.bindings {
		binding := key.NewBinding(key.WithKeys(b.keys...), key.WithHelp(strings.Join(b.keys, " | "), b.description))
		bindings = append(bindings, binding)
	}

	return help.New().FullHelpView([][]key.Binding{bindings})
}

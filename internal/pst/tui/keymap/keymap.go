package keymap

import (
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type Keymap struct {
	m    map[string]entry
	keys []string
}

type entry struct {
	description string
	cmd         tea.Cmd
	fn          func()
}

func New() *Keymap {
	return &Keymap{}
}

func (km *Keymap) AddCmd(key, description string, cmd tea.Cmd) *Keymap {
	if cmd != nil {
		km.addEntry(key, entry{
			description: description,
			cmd:         cmd,
		})
	}

	return km
}

func (km *Keymap) AddFunc(key, description string, fn func()) *Keymap {
	if fn != nil {
		km.addEntry(key, entry{
			description: description,
			fn:          fn,
		})
	}
	return km
}

func (km *Keymap) HandleKey(key tea.KeyMsg) tea.Cmd {
	e, ok := km.m[key.String()]
	if !ok {
		return nil
	}

	if e.cmd != nil {
		return e.cmd
	}

	if e.fn != nil {
		// safe-guard, shouldn't happen
		e.fn()
	}

	return nil
}

func (km *Keymap) Help() string {
	bindings := make([]key.Binding, 0, len(km.m))
	for _, k := range km.keys {
		b := key.NewBinding(key.WithKeys(k), key.WithHelp(k, km.m[k].description))
		bindings = append(bindings, b)
	}

	return help.New().FullHelpView([][]key.Binding{bindings})
}

func (km *Keymap) addEntry(key string, e entry) {
	km.keys = append(km.keys, key)

	if km.m == nil {
		km.m = make(map[string]entry)
	}

	km.m[key] = e
}

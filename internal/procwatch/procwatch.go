package procwatch

type Event struct {
	Type       EventType
	PID        int
	TID        int
	ParentPID  int
	ParentTID  int
	ExitCode   uint32
	ExitSignal uint32
}

type EventType string

const (
	TypeFork EventType = "fork"
	TypeExec EventType = "exec"
	TypeExit EventType = "exit"
)

type Watcher interface {
	Recv() (*Event, error)
	Close()
}

func Watch() (Watcher, error) {
	w, err := newWatcher()
	if err != nil {
		return nil, err
	}

	if err = w.initListen(); err != nil {
		return nil, err
	}

	go func() {
		defer w.Close()

		err := w.listen()
		if err != nil {
			w.ch <- watcherMessage{err: err}
		}
	}()

	return w, nil
}

type watcher struct {
	sock int
	ch   chan watcherMessage
}

type watcherMessage struct {
	ev  Event
	err error
}

func (w *watcher) Recv() (*Event, error) {
	msg, ok := <-w.ch
	if !ok {
		return nil, nil
	}

	return &msg.ev, msg.err
}

func (w *watcher) Close() {
	unix.Close(w.sock)
}

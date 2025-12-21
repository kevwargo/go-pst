package procwatch

import (
	"golang.org/x/sys/unix"
)

type EventFork struct {
	PID       int
	TID       int
	ParentPID int
	ParentTID int
}

type EventExec struct {
	PID int
	TID int
}

type EventExit struct {
	PID        int
	TID        int
	ParentPID  int
	ParentTID  int
	ExitCode   uint32
	ExitSignal int32
}

type Watcher interface {
	Recv() (any, error)
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
	ev  any
	err error
}

func (w *watcher) Recv() (any, error) {
	msg, ok := <-w.ch
	if !ok {
		return nil, nil
	}

	return msg.ev, msg.err
}

func (w *watcher) Close() {
	unix.Close(w.sock)
}

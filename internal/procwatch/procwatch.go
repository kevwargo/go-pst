package procwatch

import (
	"golang.org/x/sys/unix"
)

type EventForkProc struct {
	PID       int
	ParentPID int
}

type EventForkThread struct {
	PID int
	TID int
}

type EventExec struct {
	PID int
	TID int
}

type EventExitProc struct {
	PID        int
	ParentPID  int
	ExitCode   int
	ExitSignal int
}

type EventExitThread struct {
	PID int
	TID int
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
		defer unix.Close(w.sock)

		err := w.listen()
		if err != nil {
			w.msgCh <- watcherMessage{err: err}
		}
	}()

	return w, nil
}

type watcher struct {
	sock   int
	msgCh  chan watcherMessage
	doneCh chan struct{}
}

type watcherMessage struct {
	ev  any
	err error
}

func (w *watcher) Recv() (any, error) {
	select {
	case msg := <-w.msgCh:
		return msg.ev, msg.err
	case <-w.doneCh:
		return nil, nil
	}
}

func (w *watcher) Close() {
	close(w.doneCh)
	unix.Close(w.sock)
}

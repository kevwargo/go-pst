package procwatch

import (
	"sync"

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

type EventComm struct {
	PID  int
	TID  int
	Comm string
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
	Recv() Message
	Close()
}

type Message struct {
	Event any
	Err   error
	EOF   bool
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
			w.msgCh <- Message{Err: err}
		}
	}()

	return w, nil
}

type watcher struct {
	sock      int
	msgCh     chan Message
	doneCh    chan struct{}
	closeOnce sync.Once
}

func (w *watcher) Recv() Message {
	select {
	case msg := <-w.msgCh:
		return msg
	case <-w.doneCh:
		return Message{EOF: true}
	}
}

func (w *watcher) Close() {
	w.closeOnce.Do(func() {
		close(w.doneCh)
		unix.Close(w.sock)
	})
}

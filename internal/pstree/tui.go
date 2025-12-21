package pstree

import (
	"fmt"
	"strconv"

	"github.com/kevwargo/go-pst/internal/procwatch"
)

func runTUI(tree *Tree) error {
	watcher, err := procwatch.Watch()
	if err != nil {
		return err
	}

	for {
		event, err := watcher.Recv()
		if err != nil || event == nil {
			return err
		}

		switch ev := event.(type) {
		case procwatch.EventFork:
			parent := strconv.Itoa(ev.ParentPID)
			if ev.ParentTID != ev.ParentPID {
				parent += fmt.Sprintf("(%d)", ev.ParentTID)
			}
			proc := strconv.Itoa(ev.PID)
			if ev.TID != ev.PID {
				proc += fmt.Sprintf("(%d)", ev.TID)
			}

			fmt.Printf("fork %s -> %s\n", parent, proc)
		case procwatch.EventExec:
			proc := strconv.Itoa(ev.PID)
			if ev.TID != ev.PID {
				proc += fmt.Sprintf("(%d)", ev.TID)
			}
			fmt.Printf("exec %s\n", proc)
		case procwatch.EventExit:
			parent := strconv.Itoa(ev.ParentPID)
			if ev.ParentTID != ev.ParentPID {
				parent += fmt.Sprintf("(%d)", ev.ParentTID)
			}
			proc := strconv.Itoa(ev.PID)
			if ev.TID != ev.PID {
				proc += fmt.Sprintf("(%d)", ev.TID)
			}

			fmt.Printf("exit %s, code:%d, signal:%d, parent:%s\n", proc, ev.ExitCode, ev.ExitSignal, parent)
		}
	}
}

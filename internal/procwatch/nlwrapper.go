package procwatch

/*
#include <linux/netlink.h>
#include <linux/connector.h>
#include <linux/cn_proc.h>
*/
import "C"

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/kevwargo/go-pst/internal/benchmark"
	"golang.org/x/sys/unix"
)

func newWatcher() (*watcher, error) {
	sock, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM, unix.NETLINK_CONNECTOR)
	if err != nil {
		return nil, fmt.Errorf("creating netlink socket: %w", err)
	}

	addr := unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Pid:    uint32(os.Getpid()),
		Groups: C.CN_IDX_PROC,
	}
	if err := unix.Bind(sock, &addr); err != nil {
		if ce := unix.Close(sock); ce != nil {
			err = errors.Join(err, fmt.Errorf("closing netlink socket: %w", ce))
		}

		return nil, fmt.Errorf("binding netlink socket: %w", err)
	}

	return &watcher{
		sock:   sock,
		msgCh:  make(chan watcherMessage, chanSize),
		doneCh: make(chan struct{}),
	}, nil
}

func (w *watcher) initListen() error {
	header := unix.NlMsghdr{
		Len:   uint32(C.sizeof_struct_nlmsghdr + C.sizeof_struct_cn_msg + C.sizeof_enum_proc_cn_mcast_op),
		Type:  uint16(unix.NLMSG_DONE),
		Flags: 0,
		Seq:   0,
		Pid:   uint32(os.Getpid()),
	}
	cnhdr := &C.struct_cn_msg{
		id:  C.struct_cb_id{idx: C.CN_IDX_PROC, val: C.CN_VAL_PROC},
		len: C.__u16(C.sizeof_enum_proc_cn_mcast_op),
	}
	var op C.enum_proc_cn_mcast_op = C.PROC_CN_MCAST_LISTEN

	buf := bytes.NewBuffer(make([]byte, 0, header.Len))

	binary.Write(buf, binary.LittleEndian, header)
	binary.Write(buf, binary.LittleEndian, cnhdr)
	binary.Write(buf, binary.LittleEndian, op)

	destAddr := &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Pid:    0, // 0 is the kernel
		Groups: C.CN_IDX_PROC,
	}

	if err := unix.Sendto(w.sock, buf.Bytes(), 0, destAddr); err != nil {
		return fmt.Errorf("sending initializing nlmsg: %w", err)
	}

	return nil
}

func (w *watcher) listen() error {
	buf := make([]byte, recvBufSize)

	for {
		n, from, err := unix.Recvfrom(w.sock, buf, 0)
		if err != nil {
			return fmt.Errorf("receiving from nl socket: %w", err)
		}

		if err := w.processMessage(buf[:n], from); err != nil {
			return err
		}
	}
}

func (w *watcher) processMessage(buf []byte, from unix.Sockaddr) error {
	defer benchmark.Record("nl.processMessage", time.Now())

	nlFrom, ok := from.(*unix.SockaddrNetlink)
	if !ok {
		return fmt.Errorf("recvfrom returned %+v which is not (*unix.SockaddrNetlink)", from)
	}
	if nlFrom.Pid != 0 {
		return fmt.Errorf("recvfrom returned %+v which was not sent by kernel", nlFrom)
	}

	nlmessages, err := syscall.ParseNetlinkMessage(buf)
	if err != nil {
		return fmt.Errorf("parsing netlink message: %w", err)
	}

	for _, nlmsg := range nlmessages {
		switch t := nlmsgType(nlmsg.Header.Type); t {
		case nlmsgNoop:
		case nlmsgDone:
			w.deliverMessage(unsafe.Pointer(&nlmsg.Data[0]))
		default:
			return fmt.Errorf("nlmsghdr %s: 0x%x", t, nlmsg.Data)
		}
	}

	return nil
}

func (w *watcher) deliverMessage(nlmsgDataPtr unsafe.Pointer) {
	cnhdr := (*C.struct_cn_msg)(nlmsgDataPtr)
	procEvent := (*C.struct_proc_event)(
		unsafe.Pointer(uintptr(unsafe.Pointer(cnhdr)) + unsafe.Sizeof(*cnhdr)),
	)
	evType := procEventType(procEvent.what)
	dataPtr := unsafe.Pointer(&procEvent.event_data)

	switch evType {
	case procEventFork:
		data := (*C.struct_fork_proc_event)(dataPtr)
		if data.child_pid == data.child_tgid {
			w.msgCh <- watcherMessage{ev: EventForkProc{
				PID:       int(data.child_tgid),
				ParentPID: int(data.parent_tgid),
			}}
		} else {
			w.msgCh <- watcherMessage{ev: EventForkThread{
				PID: int(data.child_tgid),
				TID: int(data.child_pid),
			}}
		}
	case procEventExec:
		data := (*C.struct_exec_proc_event)(dataPtr)
		w.msgCh <- watcherMessage{ev: EventExec{
			PID: int(data.process_tgid),
			TID: int(data.process_pid),
		}}
	case procEventComm:
		data := (*C.struct_comm_proc_event)(dataPtr)
		w.msgCh <- watcherMessage{ev: EventComm{
			PID:  int(data.process_tgid),
			TID:  int(data.process_pid),
			Comm: C.GoString(&data.comm[0]),
		}}
	case procEventExit:
		data := (*C.struct_exit_proc_event)(dataPtr)
		if data.parent_pid == 0 && data.parent_tgid == 0 {
			w.msgCh <- watcherMessage{ev: EventExitThread{
				PID: int(data.process_tgid),
				TID: int(data.process_pid),
			}}
		} else {
			w.msgCh <- watcherMessage{ev: EventExitProc{
				PID:       int(data.process_tgid),
				ParentPID: int(data.parent_tgid),

				// TODO: properly parse exit_code, using WIFEXITED, etc.
				// For now it just duplicates the logic from
				// /usr/include/x86_64-linux-gnu/bits/waitstatus.h
				ExitCode:   int((data.exit_code & 0xff00) >> 8),
				ExitSignal: int(data.exit_code & 0x7f),
			}}
		}
	}
}

type nlmsgType uint16

const (
	nlmsgNoop    nlmsgType = unix.NLMSG_NOOP
	nlmsgDone    nlmsgType = unix.NLMSG_DONE
	nlmsgError   nlmsgType = unix.NLMSG_ERROR
	nlmsgOverrun nlmsgType = unix.NLMSG_OVERRUN
)

func (t nlmsgType) String() string {
	switch t {
	case nlmsgNoop:
		return "noop"
	case nlmsgDone:
		return "done"
	case nlmsgError:
		return "error"
	case nlmsgOverrun:
		return "overrun"
	default:
		return fmt.Sprintf("0x%08x", uint16(t))
	}
}

type procEventType uint32

const (
	procEventNone     procEventType = C.PROC_EVENT_NONE
	procEventFork     procEventType = C.PROC_EVENT_FORK
	procEventExec     procEventType = C.PROC_EVENT_EXEC
	procEventUid      procEventType = C.PROC_EVENT_UID
	procEventGid      procEventType = C.PROC_EVENT_GID
	procEventSid      procEventType = C.PROC_EVENT_SID
	procEventPtrace   procEventType = C.PROC_EVENT_PTRACE
	procEventComm     procEventType = C.PROC_EVENT_COMM
	procEventCoredump procEventType = C.PROC_EVENT_COREDUMP
	procEventExit     procEventType = C.PROC_EVENT_EXIT
)

func (t procEventType) String() string {
	switch t {
	case procEventNone:
		return "none"
	case procEventFork:
		return "fork"
	case procEventExec:
		return "exec"
	case procEventUid:
		return "uid"
	case procEventGid:
		return "gid"
	case procEventSid:
		return "sid"
	case procEventPtrace:
		return "ptrace"
	case procEventComm:
		return "comm"
	case procEventCoredump:
		return "coredump"
	case procEventExit:
		return "exit"
	default:
		return fmt.Sprintf("0x%08x", uint32(t))
	}
}

const (
	chanSize    = 100
	recvBufSize = 1 << 16
)

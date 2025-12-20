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
	"unsafe"

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
		sock: sock,
		ch:   make(chan watcherMessage, chanSize),
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

	return unix.Sendto(w.sock, buf.Bytes(), 0, destAddr)
}

func (w *watcher) listen() error {
	buf := make([]byte, recvBufSize)

	for {
		n, from, err := unix.Recvfrom(w.sock, buf, 0)
		if err != nil {
			return err
		}

		nlFrom, ok := from.(*unix.SockaddrNetlink)
		if !ok {
			return fmt.Errorf("recvfrom returned %+v which is not (*unix.SockaddrNetlink)", from)
		}
		if nlFrom.Pid != 0 {
			return fmt.Errorf("recvfrom returned %+v which was not sent by kernel", nlFrom)
		}

		nlmessages, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return err
		}

		for _, nlmsg := range nlmessages {
			if nlmsg.Header.Type != unix.NLMSG_DONE {
				continue
			}

			cnhdr := (*C.struct_cn_msg)(unsafe.Pointer(&nlmsg.Data[0]))
			procEvent := (*C.struct_proc_event)(
				unsafe.Pointer(uintptr(unsafe.Pointer(cnhdr)) + unsafe.Sizeof(*cnhdr)),
			)
			var msg watcherMessage

			switch procEvent.what {
			case C.PROC_EVENT_FORK:
				data := (*C.struct_fork_proc_event)(unsafe.Pointer(&procEvent.event_data))
				msg.ev = Event{
					Type:      TypeFork,
					PID:       int(data.child_tgid),
					TID:       int(data.child_pid),
					ParentPID: int(data.parent_tgid),
					ParentTID: int(data.parent_pid),
				}
			case C.PROC_EVENT_EXEC:
				data := (*C.struct_exec_proc_event)(unsafe.Pointer(&procEvent.event_data))
				msg.ev = Event{
					Type: TypeExec,
					PID:  int(data.process_tgid),
					TID:  int(data.process_pid),
				}
			case C.PROC_EVENT_EXIT:
				data := (*C.struct_exit_proc_event)(unsafe.Pointer(&procEvent.event_data))
				msg.ev = Event{
					Type:       TypeExit,
					PID:        int(data.process_tgid),
					TID:        int(data.process_pid),
					ParentPID:  int(data.parent_tgid),
					ParentTID:  int(data.parent_pid),
					ExitCode:   uint32(data.exit_code),
					ExitSignal: uint32(data.exit_signal),
				}
			default:
				msg.ev = Event{Type: EventType(fmt.Sprintf("0x%x", procEvent.what))}
			}

			w.ch <- msg
		}
	}
}

const (
	chanSize    = 100
	recvBufSize = 1 << 16
)

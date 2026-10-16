//go:build unix

package kitty

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// Probe asks the terminal (via /dev/tty, before the UI starts) whether it
// supports kitty graphics and how big a cell is. It sends a graphics query,
// a cell-size query (CSI 16 t) and DA1; every terminal answers DA1, so we
// stop reading there instead of waiting out the timeout.
func Probe(timeout time.Duration) Caps {
	var caps Caps
	fd, err := unix.Open("/dev/tty", unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return caps
	}
	defer unix.Close(fd)
	old, err := term.MakeRaw(fd)
	if err != nil {
		return caps
	}
	defer term.Restore(fd, old)

	query := "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\\x1b[16t\x1b[c"
	if _, err := unix.Write(fd, []byte(query)); err != nil {
		return caps
	}
	var reply []byte
	buf := make([]byte, 256)
	deadline := time.Now().Add(timeout)
	for !da1Reply.Match(reply) {
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if n, err := unix.Poll(fds, int(left.Milliseconds())+1); err != nil || n == 0 {
			if err == unix.EINTR {
				continue
			}
			break
		}
		n, err := unix.Read(fd, buf)
		if err != nil || n <= 0 {
			break
		}
		reply = append(reply, buf[:n]...)
	}
	return parseReply(reply)
}

// CellSize returns the terminal cell size in pixels from the window size
// ioctl, guessing 8x16 when the terminal doesn't report pixels.
func CellSize() (w, h int) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err == nil && ws.Col > 0 && ws.Row > 0 && ws.Xpixel > 0 && ws.Ypixel > 0 {
		return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row)
	}
	return 8, 16
}

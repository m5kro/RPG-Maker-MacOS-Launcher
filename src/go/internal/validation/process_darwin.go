package validation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type ownedProcess struct {
	cmd      *exec.Cmd
	port     int
	done     chan error
	waited   bool
	owned    bool
	closeErr error
}

func freePort() (int, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	return port, listener.Close()
}

func startOwned(command *exec.Cmd, port int) (*ownedProcess, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, err
	}
	p := &ownedProcess{cmd: command, port: port, done: make(chan error, 1), owned: true}
	go func() { p.done <- command.Wait(); close(p.done) }()
	return p, nil
}

func portClosed(port int) bool {
	conn, err := net.DialTimeout("tcp4", "127.0.0.1:"+strconv.Itoa(port), 100*time.Millisecond)
	if err == nil {
		conn.Close()
		return false
	}
	return errors.Is(err, syscall.ECONNREFUSED)
}

func groupGone(pid int) bool { return errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH) }

func (p *ownedProcess) close() error {
	if p.waited {
		return p.closeErr
	}
	if !p.owned || p.cmd == nil || p.cmd.Process == nil || p.cmd.Process.Pid <= 1 {
		return errors.New("refusing cleanup of unowned process group")
	}
	pid := p.cmd.Process.Pid
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		if !groupGone(pid) {
			if err := syscall.Kill(-pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
				p.closeErr = errors.Join(p.closeErr, err)
			}
		}
		deadline := time.Now().Add(3 * time.Second)
		for !groupGone(pid) && time.Now().Before(deadline) {
			time.Sleep(30 * time.Millisecond)
		}
		if groupGone(pid) {
			break
		}
	}
	select {
	case <-p.done:
		p.waited = true
	case <-time.After(3 * time.Second):
		p.closeErr = errors.Join(p.closeErr, errors.New("owned process Wait timed out"))
	}
	if !groupGone(pid) {
		p.closeErr = errors.Join(p.closeErr, fmt.Errorf("owned group %d remains", pid))
	}
	deadline := time.Now().Add(2 * time.Second)
	for !portClosed(p.port) && time.Now().Before(deadline) {
		time.Sleep(30 * time.Millisecond)
	}
	if !portClosed(p.port) {
		p.closeErr = errors.Join(p.closeErr, fmt.Errorf("debug port %d is not closed", p.port))
	}
	return p.closeErr
}

func verifyListener(ctx context.Context, p *ownedProcess, log io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-a", "-iTCP:"+strconv.Itoa(p.port), "-sTCP:LISTEN", "-Fpn")
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	b, err := cmd.CombinedOutput()
	if _, e := log.Write(b); e != nil {
		return e
	}
	if err != nil {
		return fmt.Errorf("listener inspection: %w: %s", err, b)
	}
	pid := 0
	found := false
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "p") {
			pid, err = strconv.Atoi(line[1:])
			if err != nil {
				return err
			}
		}
		if strings.HasPrefix(line, "n") {
			if line[1:] != "127.0.0.1:"+strconv.Itoa(p.port) {
				return fmt.Errorf("debug listener not exclusive IPv4 loopback: %s", line)
			}
			group, e := syscall.Getpgid(pid)
			if e != nil || group != p.cmd.Process.Pid {
				return fmt.Errorf("debug listener pid %d is not in owned group %d", pid, p.cmd.Process.Pid)
			}
			found = true
		}
	}
	if !found {
		return errors.New("no verified loopback listener")
	}
	return nil
}

package validation

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestOwnedProcessWorker(t *testing.T) {
	if os.Getenv("MZ_PROCESS_TEST") != "1" {
		return
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:"+os.Getenv("MZ_PORT"))
	if err != nil {
		os.Exit(2)
	}
	defer listener.Close()
	// Exercise escalation, not only a cooperative SIGTERM exit.
	signal.Ignore(syscall.SIGTERM)
	child := exec.Command("/bin/sleep", "60")
	if err = child.Start(); err != nil {
		os.Exit(3)
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestOwnedProcessCleanupAndListener(t *testing.T) {
	port, err := freePort()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestOwnedProcessWorker$")
	command.Env = []string{"MZ_PROCESS_TEST=1", "MZ_PORT=" + strconv.Itoa(port)}
	p, err := startOwned(command, port)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		conn, e := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 50*time.Millisecond)
		if e == nil {
			conn.Close()
			break
		}
		if e = pause(ctx, 20*time.Millisecond); e != nil {
			t.Fatal(e)
		}
	}
	if err = verifyListener(ctx, p, io.Discard); err != nil {
		t.Fatal(err)
	}
	outsider := &ownedProcess{cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}, port: port}
	if err = verifyListener(ctx, outsider, io.Discard); err == nil {
		t.Fatal("foreign listener accepted")
	}
	if err = p.close(); err != nil {
		t.Fatal(err)
	}
	if !p.waited || !portClosed(port) {
		t.Fatal("process not waited or port remains open")
	}
	if err = p.close(); err != nil {
		t.Fatal(err)
	}
}

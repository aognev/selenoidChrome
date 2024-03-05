package service

import (
	"errors"
	"fmt"
	"github.com/aerokube/selenoid/info"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aerokube/selenoid/session"
)

// Driver - driver processes manager
type Driver struct {
	ServiceBase
	Environment
	session.Caps
}

// StartWithCancel - Starter interface implementation
func (d *Driver) StartWithCancel() (*StartedService, error) {
	requestId := d.RequestId
	slice, ok := d.Service.Image.([]interface{})
	if !ok {
		return nil, fmt.Errorf("configuration error: image is not an array: %v", d.Service.Image)
	}
	var cmdLine []string
	for _, c := range slice {
		if _, ok := c.(string); !ok {
			return nil, fmt.Errorf("configuration error: value is not a string: %v", c)
		}
		cmdLine = append(cmdLine, c.(string))
	}
	if len(cmdLine) == 0 {
		return nil, errors.New("configuration error: image is empty")
	}
	log.Printf("[%d] [ALLOCATING_PORT]", requestId)
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("cannot bind to port: %v", err)
	}
	u := &url.URL{Scheme: "http", Host: l.Addr().String(), Path: d.Service.Path}
	_, port, _ := net.SplitHostPort(l.Addr().String())
	log.Printf("[%d] [ALLOCATED_PORT] [%s]", requestId, port)
	cmdLine = append(cmdLine, fmt.Sprintf("--port=%s", port))
	cmd := exec.Command(cmdLine[0], cmdLine[1:]...)
	cmd.Env = append(cmd.Env, d.Service.Env...)
	cmd.Env = append(cmd.Env, d.Caps.Env...)
	if d.CaptureDriverLogs {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else if d.LogOutputDir != "" && (d.SaveAllLogs || d.Log) {
		filename := filepath.Join(d.LogOutputDir, d.LogName)
		f, err := os.Create(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to create log file %s: %v", d.LogName, err)
		}
		cmd.Stdout = f
		cmd.Stderr = f
	}
	_ = l.Close()
	log.Printf("[%d] [STARTING_PROCESS] [%s]", requestId, cmdLine)
	s := time.Now()
	err = cmd.Start()

	if err != nil {
		return nil, fmt.Errorf("cannot start process %v: %v", cmdLine, err)
	}
	err = wait(u.String(), d.StartupTimeout)
	if err != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
		return nil, err
	}
	log.Printf("[%d] [PROCESS_STARTED] [%d] [%.2fs]", requestId, cmd.Process.Pid, info.SecondsSince(s))
	log.Printf("[%d] [PROXY_TO] [%s]", requestId, u.String())
	hp := session.HostPort{}
	if d.Caps.VNC {
		hp.VNC = "127.0.0.1:5900"
	}
	return &StartedService{Url: u, HostPort: hp, Origin: fmt.Sprintf("localhost:%s", port), Cancel: func() {
		d.stopDriver(cmd, url.URL{Scheme: "http", Host: "127.0.0.1:" + port, Path: "shutdown"})
	}}, nil
}

func (d *Driver) stopDriver(cmd *exec.Cmd, url url.URL) {
	s := time.Now()
	log.Printf("[%d] [TERMINATING_PROCESS] [%d]", d.RequestId, cmd.Process.Pid)
	deadline := time.Now().Add(10 * time.Second)
	result, err := http.Get(url.String())
	if err != nil {
		log.Printf("[%d] [GRACEFUL_CHROMEDRIVER_SHUTDOWN_FAILED] [%d] [%s]", d.RequestId, cmd.Process.Pid, err)
	}
	for {
		if result.StatusCode != 200 {
			break
		}
		if time.Now().After(deadline) {
			log.Printf("[%d] [GRACEFUL_SHUTDOWN_PROCESS_FAILED_AFTER %d seconds] [%d]", d.RequestId, 10, cmd.Process.Pid)
			break
		}
		time.Sleep(1 * time.Second)
		result, err = http.Get(url.String())
		if err != nil {
			break
		}
	}

	err = stopProc(cmd)
	if stdout, ok := cmd.Stdout.(*os.File); ok && !d.CaptureDriverLogs && d.LogOutputDir != "" {
		_ = stdout.Close()
	}
	if err != nil {
		log.Printf("[%d] [FAILED_TO_TERMINATE_PROCESS] [%d] [%v]", d.RequestId, cmd.Process.Pid, err)
		return
	}
	log.Printf("[%d] [TERMINATED_PROCESS] [%d] [%.2fs]", d.RequestId, cmd.Process.Pid, info.SecondsSince(s))
}

func stopProc(cmd *exec.Cmd) error {
	time.Sleep(2 * time.Second)
	err := cmd.Wait()
	if err != nil {
		return err
	}
	if !cmd.ProcessState.Exited() {
		err := cmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			return err
		}
	}
	return nil
}

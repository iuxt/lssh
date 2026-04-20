package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

var zmodemStartMarker = []byte{0x2a, 0x2a, 0x18, 0x42, 0x30}

type transferMode int

const (
	transferNone transferMode = iota
	transferUpload
	transferDownload
)

type sessionState struct {
	mu                sync.Mutex
	atLineStart       bool
	lineBuffer        []byte
	escapeBuffer      []byte
	escapePending     bool
	remoteProbe       []byte
	transferRequested transferMode
	transferActive    bool
	uploadFiles       []string
	remoteInput       io.Writer
	transfer          *zmodemTransfer
}

type zmodemTransfer struct {
	mode transferMode
	cmd  *exec.Cmd
	in   io.WriteCloser
	done chan error
}

func RunSSHClient(profileName string, profile SSHProfile) error {
	hostKeyCallback, err := buildHostKeyCallback(profile)
	if err != nil {
		return err
	}

	sshConfig := &ssh.ClientConfig{
		User:            profile.User,
		Auth:            []ssh.AuthMethod{ssh.Password(profile.Password)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(profile.Host, fmt.Sprintf("%d", profile.Port))
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return fmt.Errorf("connect to %s (%s) failed: %w", profileName, addr, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create ssh session: %w", err)
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("open session stdin: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open session stdout: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return fmt.Errorf("open session stderr: %w", err)
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return errors.New("stdin must be a terminal for interactive mode")
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("switch terminal to raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	width, height, err := term.GetSize(fd)
	if err != nil {
		width, height = 80, 24
	}

	if err := session.RequestPty("xterm-256color", height, width, ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}); err != nil {
		return fmt.Errorf("request pty: %w", err)
	}

	if err := session.Shell(); err != nil {
		return fmt.Errorf("start remote shell: %w", err)
	}

	state := &sessionState{
		atLineStart: true,
		remoteInput: stdin,
	}

	go watchWindowSize(fd, session)

	copyErrCh := make(chan error, 3)
	go pumpLocalInput(state, os.Stdin, copyErrCh)
	go pumpRemoteOutput(state, stdout, os.Stdout, copyErrCh)
	go pumpRemoteOutput(state, stderr, os.Stderr, copyErrCh)

	waitErr := session.Wait()

	state.finishTransfer()
	select {
	case copyErr := <-copyErrCh:
		if copyErr != nil && !errors.Is(copyErr, io.EOF) {
			return copyErr
		}
	default:
	}

	if waitErr != nil {
		var exitErr *ssh.ExitError
		if errors.As(waitErr, &exitErr) {
			return fmt.Errorf("remote exited with status %d", exitErr.ExitStatus())
		}
		return waitErr
	}

	return nil
}

func buildHostKeyCallback(profile SSHProfile) (ssh.HostKeyCallback, error) {
	if profile.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	knownHostsPath := profile.KnownHosts
	if knownHostsPath == "" {
		knownHostsPath = "~/.ssh/known_hosts"
	}
	expanded, err := expandHome(knownHostsPath)
	if err != nil {
		return nil, err
	}
	return knownhosts.New(expanded)
}

func expandHome(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func watchWindowSize(fd int, session *ssh.Session) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGWINCH)
	defer signal.Stop(signals)

	for range signals {
		width, height, err := term.GetSize(fd)
		if err == nil {
			_ = session.WindowChange(height, width)
		}
	}
}

func pumpLocalInput(state *sessionState, in io.Reader, errCh chan<- error) {
	buf := make([]byte, 1)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			if writeErr := state.handleLocalByte(buf[0]); writeErr != nil {
				errCh <- writeErr
				return
			}
		}
		if err != nil {
			errCh <- err
			return
		}
	}
}

func pumpRemoteOutput(state *sessionState, src io.Reader, dst io.Writer, errCh chan<- error) {
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if writeErr := state.handleRemoteBytes(buf[:n], dst); writeErr != nil {
				errCh <- writeErr
				return
			}
		}
		if err != nil {
			errCh <- err
			return
		}
	}
}

func (s *sessionState) handleLocalByte(b byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.transferActive {
		return nil
	}

	if s.escapePending {
		return s.handleEscapeByteLocked(b)
	}

	if s.atLineStart && b == '~' {
		s.escapePending = true
		s.escapeBuffer = []byte{'~'}
		_, _ = os.Stdout.Write([]byte{'~'})
		return nil
	}

	if err := s.writeRemoteLocked([]byte{b}); err != nil {
		return err
	}
	s.trackLineLocked(b)
	return nil
}

func (s *sessionState) handleEscapeByteLocked(b byte) error {
	s.escapeBuffer = append(s.escapeBuffer, b)

	if b == '\r' || b == '\n' {
		line := strings.TrimSpace(string(s.escapeBuffer))
		s.escapePending = false
		s.escapeBuffer = nil
		s.atLineStart = true

		switch {
		case line == "~.":
			return io.EOF
		case strings.HasPrefix(line, "~rz "):
			args := strings.Fields(line)
			if len(args) < 2 {
				_, _ = os.Stderr.WriteString("\r\nusage: ~rz <local-file> [more-files...]\r\n")
				return nil
			}
			s.transferRequested = transferUpload
			s.uploadFiles = append([]string(nil), args[1:]...)
			_, _ = os.Stderr.WriteString("\r\nstarting remote rz...\r\n")
			return s.writeRemoteLocked([]byte("rz\n"))
		case line == "~rz":
			_, _ = os.Stderr.WriteString("\r\nusage: ~rz <local-file> [more-files...]\r\n")
			return nil
		default:
			return s.writeRemoteLocked(s.escapeBufferWithNewline(line))
		}
	}

	_, _ = os.Stdout.Write([]byte{b})
	return nil
}

func (s *sessionState) escapeBufferWithNewline(line string) []byte {
	if line == "" {
		return []byte("\n")
	}
	return []byte(line + "\n")
}

func (s *sessionState) writeRemoteLocked(p []byte) error {
	_, err := s.remoteInput.Write(p)
	return err
}

func (s *sessionState) trackLineLocked(b byte) {
	switch b {
	case '\r', '\n':
		line := strings.TrimSpace(string(s.lineBuffer))
		s.atLineStart = true
		s.lineBuffer = s.lineBuffer[:0]
		if strings.HasPrefix(line, "sz ") || line == "sz" {
			s.transferRequested = transferDownload
		} else {
			s.transferRequested = transferNone
			s.uploadFiles = nil
		}
	case 0x7f, 0x08:
		if len(s.lineBuffer) > 0 {
			s.lineBuffer = s.lineBuffer[:len(s.lineBuffer)-1]
		}
		s.atLineStart = len(s.lineBuffer) == 0
	default:
		if b >= 0x20 && b != 0x7f {
			s.lineBuffer = append(s.lineBuffer, b)
			s.atLineStart = false
		}
	}
}

func (s *sessionState) handleRemoteBytes(p []byte, dst io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.transferActive && s.transfer != nil {
		_, err := s.transfer.in.Write(p)
		return err
	}

	if s.transferRequested != transferNone && bytes.Contains(p, zmodemStartMarker) {
		if err := s.startTransferLocked(); err != nil {
			return err
		}
		if s.transfer != nil {
			_, err := s.transfer.in.Write(p)
			return err
		}
	}

	if s.transferRequested != transferNone {
		s.remoteProbe = append(s.remoteProbe, p...)
		if len(s.remoteProbe) > 64 {
			s.remoteProbe = append([]byte(nil), s.remoteProbe[len(s.remoteProbe)-64:]...)
		}
		if bytes.Contains(s.remoteProbe, zmodemStartMarker) {
			if err := s.startTransferLocked(); err != nil {
				return err
			}
			if s.transfer != nil {
				_, err := s.transfer.in.Write(p)
				return err
			}
		}
	} else {
		s.remoteProbe = s.remoteProbe[:0]
	}

	_, err := dst.Write(p)
	return err
}

func (s *sessionState) startTransferLocked() error {
	if s.transferActive {
		return nil
	}

	var cmd *exec.Cmd
	switch s.transferRequested {
	case transferUpload:
		if len(s.uploadFiles) == 0 {
			return errors.New("upload requested but no local files were provided")
		}
		args := append([]string{"-e", "-b"}, s.uploadFiles...)
		cmd = exec.Command("sz", args...)
		_, _ = os.Stderr.WriteString("\r\nuploading via local sz...\r\n")
	case transferDownload:
		cmd = exec.Command("rz", "-E", "-y", "-b")
		_, _ = os.Stderr.WriteString("\r\nreceiving via local rz...\r\n")
	default:
		return nil
	}

	in, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open local lrzsz stdin: %w", err)
	}
	cmd.Stdout = writerFunc(func(p []byte) (int, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.remoteInput.Write(p)
	})
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start local %s: %w", cmd.Path, err)
	}

	transfer := &zmodemTransfer{
		mode: s.transferRequested,
		cmd:  cmd,
		in:   in,
		done: make(chan error, 1),
	}
	s.transfer = transfer
	s.transferActive = true
	s.remoteProbe = s.remoteProbe[:0]

	go func() {
		err := cmd.Wait()
		_ = in.Close()
		transfer.done <- err
	}()

	go s.waitTransfer(transfer)
	return nil
}

func (s *sessionState) waitTransfer(transfer *zmodemTransfer) {
	err := <-transfer.done
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.transfer == transfer {
		s.transferActive = false
		s.transferRequested = transferNone
		s.uploadFiles = nil
		s.transfer = nil
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\r\nzmodem transfer ended with error: %v\r\n", err)
	} else {
		_, _ = os.Stderr.WriteString("\r\nzmodem transfer completed.\r\n")
	}
}

func (s *sessionState) finishTransfer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.transfer != nil && s.transfer.in != nil {
		_ = s.transfer.in.Close()
	}
}

type writerFunc func(p []byte) (int, error)

func (w writerFunc) Write(p []byte) (int, error) {
	return w(p)
}

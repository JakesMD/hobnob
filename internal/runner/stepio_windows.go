//go:build windows

package runner

import (
	"context"
	"errors"
	"io"
	"os"
	osExec "os/exec"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ptyPassthrough is CreatePseudoConsole's PSEUDOCONSOLE_PASSTHROUGH_MODE: the
// output is the command's own VT stream. Without it ConPTY redraws a screen
// buffer, positioning the cursor for every line, which an append-only log
// can't take. Windows versions that lack it reject the flag, and then the
// step gets plain pipes.
const ptyPassthrough = 0x8

// conPTY is a pseudo console as one step's stdout. Only stdout is attached to
// it: stderr is a pipe and stdin is hobnob's own, both handed to the process
// explicitly in startOnPTY.
type conPTY struct {
	hpc     windows.Handle
	input   *os.File // keystrokes to the pseudo console; also the registry key
	drained chan struct{}

	closeOnce sync.Once
	mu        sync.Mutex
	process   windows.Handle // the step's process, once started
}

// conPTYs maps a step's master (the pseudo console's input pipe) to its conPTY,
// which keeps the seam shared with Unix: a *os.File says which PTY to resize.
var conPTYs sync.Map

func conPTYOf(master *os.File) *conPTY {
	if master == nil {
		return nil
	}
	cp, _ := conPTYs.Load(master)
	c, _ := cp.(*conPTY)
	return c
}

func newPTYPipe(size ptySize) (*outPipe, *os.File, error) {
	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return nil, nil, err
	}
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		windows.CloseHandle(inRead)
		windows.CloseHandle(inWrite)
		return nil, nil, err
	}
	var hpc windows.Handle
	err := windows.CreatePseudoConsole(coordOf(size), inRead, outWrite, ptyPassthrough, &hpc)
	// The pseudo console holds its own copies of these two ends.
	windows.CloseHandle(inRead)
	windows.CloseHandle(outWrite)
	if err != nil {
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		return nil, nil, err
	}
	// attachPipes and startCopying expect a write end to close once the
	// child has its own; the pseudo console's side needs none.
	unused, err := os.Open(os.DevNull)
	if err != nil {
		windows.ClosePseudoConsole(hpc)
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		return nil, nil, err
	}
	cp := &conPTY{hpc: hpc, input: os.NewFile(uintptr(inWrite), "conin"), drained: make(chan struct{})}
	conPTYs.Store(cp.input, cp)
	return &outPipe{r: os.NewFile(uintptr(outRead), "conout"), w: unused}, cp.input, nil
}

func coordOf(size ptySize) windows.Coord {
	return windows.Coord{X: int16(size.cols), Y: int16(size.rows)}
}

func setPTYSize(master *os.File, size ptySize) {
	if cp := conPTYOf(master); cp != nil {
		_ = windows.ResizePseudoConsole(cp.hpc, coordOf(size))
	}
}

// startOnPTY creates the step's process attached to its pseudo console. os/exec
// can't: it has no way to pass the attribute, so the process is created here
// and handed to cmd for Wait.
func (p *stepPipes) startOnPTY(ctx context.Context, cmd *osExec.Cmd) error {
	cp := conPTYOf(p.master)
	if cp == nil {
		return cmd.Start()
	}
	inherited, closeInherited, err := inheritableStdio(p.stderr.w)
	if err != nil {
		return err
	}
	defer closeInherited()

	attrs, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return err
	}
	defer attrs.Delete()
	// The attribute's value is the HPCON itself, not a pointer to it.
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&cp.hpc)), unsafe.Sizeof(cp.hpc)); err != nil {
		return err
	}
	handles := []windows.Handle{inherited.stdin, inherited.stderr}
	if inherited.stdin == 0 {
		handles = handles[1:]
	}
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return err
	}

	startup := &windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	startup.Cb = uint32(unsafe.Sizeof(*startup))
	// A zero stdout handle is filled in by the pseudo console.
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput, startup.StdErr = inherited.stdin, inherited.stderr

	argv0, err := windows.UTF16PtrFromString(cmd.Path)
	if err != nil {
		return err
	}
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(cmd.Args))
	if err != nil {
		return err
	}
	var dir *uint16
	if cmd.Dir != "" {
		if dir, err = windows.UTF16PtrFromString(cmd.Dir); err != nil {
			return err
		}
	}
	env, err := environmentBlock(cmd.Env)
	if err != nil {
		return err
	}
	var info windows.ProcessInformation
	err = windows.CreateProcess(argv0, cmdline, nil, nil, true,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT,
		env, dir, &startup.StartupInfo, &info)
	if err != nil {
		return &os.PathError{Op: "CreateProcess", Path: cmd.Path, Err: err}
	}
	windows.CloseHandle(info.Thread)
	proc, err := os.FindProcess(int(info.ProcessId))
	if err != nil {
		windows.TerminateProcess(info.Process, 1)
		windows.CloseHandle(info.Process)
		return err
	}
	cp.mu.Lock()
	cp.process = info.Process
	cp.mu.Unlock()
	cmd.Process = proc
	go cp.interruptOnCancel(ctx)
	return nil
}

type stdioHandles struct{ stdin, stderr windows.Handle }

// inheritableStdio returns the handles a ConPTY step gets for stdin and
// stderr: hobnob's own stdin, so prompts reach the command, and stderr's
// pipe. close releases the duplicates once the process has them.
func inheritableStdio(stderr *os.File) (stdioHandles, func(), error) {
	var h stdioHandles
	var toClose []windows.Handle
	closeAll := func() {
		for _, dup := range toClose {
			windows.CloseHandle(dup)
		}
	}
	self := windows.CurrentProcess()
	dup := func(src windows.Handle) (windows.Handle, error) {
		var out windows.Handle
		err := windows.DuplicateHandle(self, src, self, &out, 0, true, windows.DUPLICATE_SAME_ACCESS)
		if err == nil {
			toClose = append(toClose, out)
		}
		return out, err
	}
	var err error
	if h.stderr, err = dup(windows.Handle(stderr.Fd())); err != nil {
		return h, closeAll, err
	}
	if stdin := windows.Handle(os.Stdin.Fd()); stdin != 0 && stdin != windows.InvalidHandle {
		if h.stdin, err = dup(stdin); err != nil {
			closeAll()
			return h, func() {}, err
		}
	}
	return h, closeAll, nil
}

// environmentBlock is env as CreateProcess takes it.
func environmentBlock(env []string) (*uint16, error) {
	if len(env) == 0 {
		return nil, nil
	}
	var block []uint16
	for _, kv := range env {
		if strings.IndexByte(kv, 0) >= 0 {
			return nil, errors.New("environment variable contains a NUL")
		}
		u, err := windows.UTF16FromString(kv)
		if err != nil {
			return nil, err
		}
		block = append(block, u...)
	}
	block = append(block, 0)
	return &block[0], nil
}

// interruptOnCancel is a ConPTY step's first CTRL+C: the pseudo console turns
// the byte into CTRL_C_EVENT for the process, as a terminal would. A command
// that ignores it is stopped by the second one, through KillRunningStep.
func (c *conPTY) interruptOnCancel(ctx context.Context) {
	select {
	case <-ctx.Done():
		c.input.Write([]byte{0x03})
	case <-c.drained:
	}
}

// terminate force-kills the step's process and takes the pseudo console, and
// so every process attached to it, down.
func (c *conPTY) terminate() {
	c.mu.Lock()
	if c.process != 0 {
		windows.TerminateProcess(c.process, 1)
	}
	c.mu.Unlock()
	c.close()
}

// close closes the pseudo console without waiting: ClosePseudoConsole can block
// until its output is read, and the copy goroutine is what reads it.
func (c *conPTY) close() {
	c.closeOnce.Do(func() {
		go func() {
			windows.ClosePseudoConsole(c.hpc)
			c.input.Close()
			conPTYs.Delete(c.input)
			c.mu.Lock()
			if c.process != 0 {
				windows.CloseHandle(c.process)
				c.process = 0
			}
			c.mu.Unlock()
		}()
	})
}

// closePTY runs once the step's process has exited. ConPTY ends its output only
// when it is closed, not when its last process exits.
func (p *stepPipes) closePTY() {
	if cp := conPTYOf(p.master); cp != nil {
		cp.close()
	}
}

func (p *stepPipes) waitPTY(d time.Duration) {
	cp := conPTYOf(p.master)
	if cp == nil {
		return
	}
	select {
	case <-cp.drained:
	case <-time.After(d):
	}
}

// filterPTY wraps the stdout copy of a ConPTY step; done reports it finished.
func (p *stepPipes) filterPTY(pipe *outPipe, w io.Writer) (io.Writer, func()) {
	cp := conPTYOf(p.master)
	if cp == nil || pipe != p.stdout {
		return w, func() {}
	}
	filter := newConPTYFilter(w)
	return filter, func() {
		filter.Flush()
		close(cp.drained)
	}
}

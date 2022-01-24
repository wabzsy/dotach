package dotach

import (
	"fmt"
	"log"
	"os"
	"syscall"
)

func NewTracer(proc *os.Process) *Tracer {
	return &Tracer{
		proc: proc,
	}
}

func (t *Tracer) Syscall(sysNo int, a1, a2, a3, a4, a5, a6 int) (int, error) {
	log.Printf("Syscall(0x%x, 0x%x, 0x%x, 0x%x, 0x%x, 0x%x, 0x%x)", uint64(sysNo), uint64(a1), uint64(a2), uint64(a3), uint64(a4), uint64(a5), uint64(a6))
	// 确保已经准备好进行syscall
	if err := t.WantState(StateBeforeSyscall); err != nil {
		return 0, err
	}

	registers := NewRegister()

	if err := t.GetRegister(registers); err != nil {
		return 0, err
	}

	//log.Printf("系统调用前的寄存器: %#v", registers)

	if err := t.setSyscallArgs(sysNo, a1, a2, a3, a4, a5, a6, registers); err != nil {
		return 0, err
	}

	//log.Printf("系统正要调用的寄存器: %#v", registers)

	if err := t.SetRegister(registers); err != nil {
		return 0, err
	}

	if err := t.WantState(StateAfterSyscall); err != nil {
		return 0, err
	}

	if err := t.GetRegister(registers); err != nil {
		return 0, err
	}

	//log.Printf("系统调用后的寄存器: %#v", registers)

	// 把寄存器恢复成原来的鸟样
	if err := t.RestoreRegister(); err != nil {
		return 0, err
	}

	if result := t.getSyscallResult(registers); result < 0 {
		return result, syscall.Errno(-result)
	} else {
		return result, nil
	}

}

func (t *Tracer) Munmap(addr uintptr) (int, error) {
	log.Printf("Munmap...")
	return t.Syscall(syscall.SYS_MUNMAP, int(addr), syscall.Getpagesize(), 0, 0, 0, 0)
}

func (t *Tracer) Mmap() (uintptr, error) {
	log.Printf("Mmap...")

	result, err := t.Syscall(syscall.SYS_MMAP,
		0,
		syscall.Getpagesize(),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANONYMOUS|syscall.MAP_PRIVATE,
		0,
		0,
	)
	if err != nil {
		return 0, err
	}

	// TODO 错误处理 返回的 scratch page 不一定是合法的地址

	log.Printf("Allocated scratch page: 0x%x", result)

	return uintptr(result), nil
}

func (t *Tracer) Memcpy(addr uintptr, str string) (int, error) {
	log.Printf("Memcpy(0x%x, %s)", addr, str)

	return syscall.PtracePokeData(t.proc.Pid, addr, []byte(str))
}

// 弃用,arm64不支持open,使用openat代替
// 参考文献: https://chromium.googlesource.com/chromiumos/docs/+/HEAD/constants/syscalls.md
//func (d *Tracer) Open(addr uintptr) (int, error) {
//	log.Printf("Open(0x%x)", addr)
//
//	return d.Syscall(syscall.SYS_OPEN, int(addr), syscall.O_RDWR|syscall.O_CREAT, 0666, 0, 0, 0)
//}

func (t *Tracer) OpenFile(filepath string) (int, error) {
	log.Printf("OpenFile(%s)", filepath)

	scratchPage, err := t.Mmap()
	if err != nil {
		return 0, err
	}

	defer func() {
		if _, err := t.Munmap(scratchPage); err != nil {
			log.Printf("Failed to free memory page: %v", err)
		} else {
			log.Printf("Scratch page freed: 0x%x", scratchPage)
		}
	}()

	n, err := t.Memcpy(scratchPage, filepath)
	if err != nil {
		return 0, err
	}

	if n != len(filepath) {
		return 0, fmt.Errorf("tracee memcpy failed: %d bytes should be written, %d bytes are actually written", len(filepath), n)
	}

	if fd, err := t.OpenAt(scratchPage); err != nil {
		return 0, err
	} else {
		//log.Printf("远程已打开文件描述符: %#v", fd)
		return fd, nil
	}
}

func (t *Tracer) OpenAt(addr uintptr) (int, error) {
	log.Printf("OpenAt(0x%x)", addr)

	return t.Syscall(syscall.SYS_OPENAT, -1, int(addr), syscall.O_RDWR|syscall.O_NOCTTY, 0, 0, 0)
}

func (t *Tracer) Close(fd int) (int, error) {
	log.Printf("Close(0x%x)", fd)

	return t.Syscall(syscall.SYS_CLOSE, fd, 0, 0, 0, 0, 0)
}

func (t *Tracer) Dup(oldFd int) (int, error) {
	log.Printf("Dup(0x%x)", oldFd)

	return t.Syscall(syscall.SYS_DUP, oldFd, 0, 0, 0, 0, 0)
}

func (t *Tracer) Dup3(oldFd, newFd int) (int, error) {
	log.Printf("Dup3(0x%x, 0x%x)", oldFd, newFd)

	return t.Syscall(syscall.SYS_DUP3, oldFd, newFd, 0, 0, 0, 0)
}

func (t *Tracer) WantState(want TraceeState) error {

	log.Printf("WantState(Current: %s, Want: %s)", t.traceeState, want)
	defer func() {
		log.Printf("WantState(Get: %s)", t.traceeState)
	}()

	if t.traceeState == want {
		return nil
	}

	for t.traceeState != want {
		switch want {
		case StateBeforeSyscall, StateAfterSyscall:
			// 想要系统调用
			if err := syscall.PtraceSyscall(t.proc.Pid, 0); err != nil {
				Debug(err)
				return err
			}
		case StateRunning:
			// 想要程序继续运行
			return syscall.PtraceCont(t.proc.Pid, 0)
		case StateStopped:
			// 想要程序停止
			if err := t.proc.Signal(syscall.SIGTSTP); err != nil {
				Debug(err)
				return err
			}
		default:
			return fmt.Errorf("the want state is wrong")
		}

		if state, err := t.Wait(); err != nil {
			Debug(err)
			return err
		} else if state == StateAtSyscall {
			// 当返回的状态是由PTRACE_SYSCALL触发的, 那么当前状态要交错着来
			if t.traceeState == StateBeforeSyscall {
				t.traceeState = StateAfterSyscall
			} else {
				t.traceeState = StateBeforeSyscall
			}
		} else {
			t.traceeState = state
		}

	}
	return nil
}

func (t *Tracer) Wait() (TraceeState, error) {
	log.Printf("Waiting...")

	var waitStatus syscall.WaitStatus

	if _, err := syscall.Wait4(t.proc.Pid, &waitStatus, 0, nil); err != nil {
		return StateUnknown, err
	}

	log.Printf("Wait Status: 0x%x", waitStatus)

	if waitStatus.Exited() {
		return StateExited, fmt.Errorf("error: Exited(status: %d)", waitStatus.ExitStatus())
	} else if waitStatus.Signaled() {
		if waitStatus.CoreDump() {
			return StateSignaled, fmt.Errorf("error: CoreDump(0x%x)", waitStatus)
		} else {
			return StateSignaled, fmt.Errorf("error: Signald(%s: %d)", waitStatus.Signal().String(), waitStatus.Signal())
		}
	} else if waitStatus.Continued() {
		return StateContinued, fmt.Errorf("error: Continued(0x%x)", waitStatus)
	} else if waitStatus.Stopped() {
		log.Printf("Stopped(%s: %d)", waitStatus.StopSignal().String(), waitStatus.StopSignal())

		switch waitStatus.StopSignal() {
		case syscall.SIGSEGV:
			return StateStopped, fmt.Errorf("error: Stopped(%s: %d)", waitStatus.StopSignal().String(), waitStatus.StopSignal())
		case syscall.SIGTRAP:
			if waitStatus.TrapCause() == 0 {
				// 一般情况下仅在attach的时候走下面的流程,也就是一般只有刚attach的时候会 SIGTRAP
				// 通常是amd64会触发
				return StateStopped, nil
			}
			log.Printf("Trapped(0x%x)", waitStatus.TrapCause())
			if waitStatus.TrapCause() == syscall.PTRACE_EVENT_FORK {
				forkedPid, err := syscall.PtraceGetEventMsg(t.proc.Pid)
				if err != nil {
					return StateTrapped, err
				}
				log.Printf("forkedPid: %d", forkedPid)
			}
			return StateTrapped, nil
		case syscall.SIGCONT:
			return StateContinued, nil
		case syscall.SIGSTOP:
			// 一般情况下仅在attach的时候走下面的流程,也就是一般只有刚attach的时候会 SIGSTOP
			// 通常是arm64会触发
			return StateStopped, nil
		default:
			// 从PTRACE_SYSCALL来的信号
			if int(waitStatus.StopSignal()&0x80) != 0 {
				return StateAtSyscall, nil
			}
			return StateStopped, fmt.Errorf("error: Unhandled(%s: %d)", waitStatus.StopSignal().String(), waitStatus.StopSignal())
		}
	} else {
		return StateUnknown, fmt.Errorf("error: Unknown wait status (0x%x)", waitStatus)
	}
}

func (t *Tracer) Attach() error {
	log.Printf("Attaching...")
	// 附加(会挂起进程)
	if err := syscall.PtraceAttach(t.proc.Pid); err != nil {
		Debug(err)
		return err
	}

	if state, err := t.Wait(); err != nil {
		Debug(err)
		return err
	} else if state != StateStopped {
		Debug(state.String())
		return fmt.Errorf("state error(want: %s, current:%s)", StateStopped, state)
	}

	if err := syscall.PtraceSetOptions(t.proc.Pid, syscall.PTRACE_O_TRACESYSGOOD|syscall.PTRACE_O_TRACEFORK); err != nil {
		Debug(err)
		return err
	}

	// TODO 未处理32位代码运行在64位CPU的情况(意思就是说 x86_64下没有判断CS=0x23还是0x33)
	if err := t.SaveRegister(); err != nil {
		Debug(err)
		return err
	}

	log.Printf("Attached.")
	return nil
}

func (t *Tracer) Detach() error {
	log.Println("Detaching...")
	// TODO: 还原寄存器
	//log.Printf("Tracee State: %s", d.state)
	//if err := d.WantState(StateAfterSyscall); err != nil {
	//	return err
	//}
	if err := syscall.PtraceDetach(t.proc.Pid); err != nil {
		return err
	}
	t.traceeState = StateDetached
	log.Println("Detached.")
	return nil
}

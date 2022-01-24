package dotach

import (
	"golang.org/x/sys/unix"
	"log"
	"os"
	"unsafe"
)

type Tracer struct {
	proc        *os.Process
	registers   *unix.PtraceRegsArm64
	traceeState TraceeState
	savedSysNo  *int
}

// 参考文献:
// (1) https://man7.org/linux/man-pages/man2/ptrace.2.html
// (2) https://patchwork.kernel.org/project/linux-arm-kernel/patch/1416273038-15590-2-git-send-email-takahiro.akashi@linaro.org/

const (
	NT_ARM_SYSTEM_CALL = 0x404
	NT_PRSTATUS        = 0x01
)

// ptrace 改编自 golang.org/x/sys/unix/zsyscall_linux.go
func ptrace(request int, pid int, addr uintptr, data uintptr) (err error) {
	_, _, e1 := unix.Syscall6(unix.SYS_PTRACE, uintptr(request), uintptr(pid), uintptr(addr), uintptr(data), 0, 0)
	if e1 != 0 {
		err = e1
	}
	return
}

// PtraceGetRegSetArm64 改编自 golang.org/x/sys/unix/zptrace_linux_arm64.go 解决了不能自定义Iovec的问题
func PtraceGetRegSetArm64(pid, addr int, iovec unix.Iovec) error {
	return ptrace(unix.PTRACE_GETREGSET, pid, uintptr(addr), uintptr(unsafe.Pointer(&iovec)))
}

// PtraceSetRegSetArm64 改编自 golang.org/x/sys/unix/zptrace_linux_arm64.go 解决了不能自定义Iovec的问题
func PtraceSetRegSetArm64(pid, addr int, iovec unix.Iovec) error {
	return ptrace(unix.PTRACE_SETREGSET, pid, uintptr(addr), uintptr(unsafe.Pointer(&iovec)))
}

func (t *Tracer) GetRegister(out *unix.PtraceRegsArm64) error {
	//defer log.Printf("GetRegister %#v", out)
	iovec := unix.Iovec{Base: (*byte)(unsafe.Pointer(out)), Len: uint64(unsafe.Sizeof(*out))}
	return PtraceGetRegSetArm64(t.proc.Pid, NT_PRSTATUS, iovec)
}

func (t *Tracer) SetRegister(in *unix.PtraceRegsArm64) error {
	//defer log.Printf("SetRegister %#v", in)
	iovec := unix.Iovec{Base: (*byte)(unsafe.Pointer(in)), Len: uint64(unsafe.Sizeof(*in))}
	return PtraceSetRegSetArm64(t.proc.Pid, NT_PRSTATUS, iovec)
}

func (t *Tracer) GetSyscallRegister(out *int) error {
	//defer log.Printf("GetSyscallRegister %#v", out)
	iovec := unix.Iovec{Base: (*byte)(unsafe.Pointer(out)), Len: uint64(unsafe.Sizeof(*out))}
	return PtraceGetRegSetArm64(t.proc.Pid, NT_ARM_SYSTEM_CALL, iovec)
}

func (t *Tracer) SetSyscallRegister(in *int) error {
	//defer log.Printf("SetSyscallRegister %#v", in)
	iovec := unix.Iovec{Base: (*byte)(unsafe.Pointer(in)), Len: uint64(unsafe.Sizeof(*in))}
	return PtraceSetRegSetArm64(t.proc.Pid, NT_ARM_SYSTEM_CALL, iovec)
}

// FixupRegisters arm和arm64都需要pc-4
func (t *Tracer) FixupRegisters() {
	t.registers.Pc -= 4
}

func (t *Tracer) SaveRegister() error {
	log.Printf("Saving registers...")
	if err := t.WantState(StateBeforeSyscall); err != nil {
		return err
	}

	t.registers = NewRegister()

	if err := t.GetRegister(t.registers); err != nil {
		return err
	}

	t.savedSysNo = new(int)
	if err := t.GetSyscallRegister(t.savedSysNo); err != nil {
		return err
	}

	defer func() {
		log.Printf("Registers saved.")
	}()

	//log.Printf("原始寄存器: %#v", d.registers)

	// 修正(回退)要保存的寄存器
	t.FixupRegisters()

	//log.Printf("修正后寄存器: %#v", d.registers)
	return nil
}

func (t *Tracer) RestoreRegister() error {
	if err := t.SetRegister(t.registers); err != nil {
		return err
	}
	if err := t.SetSyscallRegister(t.savedSysNo); err != nil {
		return err
	}
	return nil
}

func (t *Tracer) setSyscallArgs(sysNo int, a1, a2, a3, a4, a5, a6 int, out *unix.PtraceRegsArm64) error {
	// 系统调用号在x8寄存器,x0-x7用于存放函数参数
	// 参考文章:
	// https://blog.csdn.net/yusakul/article/details/105706674
	// https://www.jianshu.com/p/cf29fb303bdc
	// https://blog.csdn.net/qq_24622489/article/details/89161125
	out.Regs[8] = uint64(sysNo)

	// linux 函数传参寄存器顺序 RDI,RSI,RDX,R10,R8,R9
	out.Regs[0] = uint64(a1)
	out.Regs[1] = uint64(a2)
	out.Regs[2] = uint64(a3)
	out.Regs[3] = uint64(a4)
	out.Regs[4] = uint64(a5)
	out.Regs[5] = uint64(a6)

	return t.SetSyscallRegister(&sysNo) // 用于解决存在于arm64特定的问题 见参考文献(2)
}

func (t *Tracer) getSyscallResult(in *unix.PtraceRegsArm64) int {
	// syscall返回结果保存在x0寄存器中
	return int(in.Regs[0])
}

func NewRegister() *unix.PtraceRegsArm64 {
	return &unix.PtraceRegsArm64{}
}

package dotach

import (
	"golang.org/x/sys/unix"
	"log"
	"os"
)

type Tracer struct {
	proc        *os.Process
	registers   *unix.PtraceRegs
	traceeState TraceeState
}

func (t *Tracer) GetRegister(out *unix.PtraceRegs) error {
	//defer log.Printf("GetRegister %#v", out)
	return unix.PtraceGetRegs(t.proc.Pid, out)
}

func (t *Tracer) SetRegister(in *unix.PtraceRegs) error {
	//defer log.Printf("SetRegister %#v", in)
	return unix.PtraceSetRegs(t.proc.Pid, in)
}

// FixupRegisters pc=rip-2 rax回退
func (t *Tracer) FixupRegisters() {
	t.registers.Rip -= 2
	t.registers.Rax = t.registers.Orig_rax
}

func (t *Tracer) SaveRegister() error {
	log.Printf("Saving registers...")
	if err := t.WantState(StateBeforeSyscall); err != nil {
		return err
	}

	defer func() {
		log.Printf("Registers saved.")
	}()

	t.registers = NewRegister()

	if err := t.GetRegister(t.registers); err != nil {
		return err
	}

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
	return nil
}

func (t *Tracer) setSyscallArgs(sysNo int, a1, a2, a3, a4, a5, a6 int, out *unix.PtraceRegs) error {
	// 由于内核的内部用途, 系统调用号是保存在 orig_rax 中而不是 rax 中 (mmp!!!)
	// 原因参考: https://zhuanlan.zhihu.com/p/42898266
	out.Orig_rax = uint64(sysNo)

	// linux x86_64 函数传参的寄存器顺序 RDI,RSI,RDX,R10,R8,R9
	out.Rdi = uint64(a1)
	out.Rsi = uint64(a2)
	out.Rdx = uint64(a3)
	out.R10 = uint64(a4)
	out.R8 = uint64(a5)
	out.R9 = uint64(a6)

	return nil
}

func (t *Tracer) getSyscallResult(in *unix.PtraceRegs) int {
	// syscall返回结果保存在rax寄存器中
	return int(in.Rax)
}

func NewRegister() *unix.PtraceRegs {
	return &unix.PtraceRegs{}
}

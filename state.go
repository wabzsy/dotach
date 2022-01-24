package dotach

type TraceeState int

const (
	StateUnknown TraceeState = iota
	StateDetached
	StateAttached
	StateAtSyscall
	StateBeforeSyscall
	StateAfterSyscall
	StateTrapped
	StateRunning
	StateStopped
	StateExited
	StateSignaled
	StateContinued
)

func (s TraceeState) String() string {
	switch s {
	case StateDetached:
		return "STATE_DETACHED"
	case StateAttached:
		return "STATE_ATTACHED"
	case StateAtSyscall:
		return "STATE_AT_SYSCALL"
	case StateBeforeSyscall:
		return "STATE_BEFORE_SYSCALL"
	case StateAfterSyscall:
		return "STATE_AFTER_SYSCALL"
	case StateTrapped:
		return "STATE_TRAPPED"
	case StateRunning:
		return "STATE_RUNNING"
	case StateStopped:
		return "STATE_STOPPED"
	case StateExited:
		return "STATE_EXITED"
	case StateSignaled:
		return "STATE_SIGNALED"
	case StateContinued:
		return "STATE_CONTINUED"
	default:
		return "STATE_UNKNOWN"
	}
}

package dotach

import (
	"fmt"
	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
	"log"
	"os"
)

// Terminal // 不能用 github.com/pkg/term/termios 的那个包 那个包有bug 会导致 MakeRaw 失败, 需要自己调用unix.Ioctl* 来获取和设置termios
type Terminal struct {
	pts *os.File
	ptm *os.File
}

// SetTermios 设置tty属性(不然不能Ctrl+C之类的)
func (t *Terminal) SetTermios(tio *unix.Termios) error {
	f, err := os.OpenFile(t.pts.Name(), os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	return unix.IoctlSetTermios(int(f.Fd()), uint(unix.TCSETS), tio) //termios.Tcsetattr(f.Fd(), termios.TCSANOW, tio)
}

func (t *Terminal) GetFileTermios(path string) (*unix.Termios, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()
	return t.GetTermios(f)
}

// GetTermios 获取tty属性
func (t *Terminal) GetTermios(file *os.File) (*unix.Termios, error) {
	if term.IsTerminal(int(file.Fd())) {
		if tio, err := unix.IoctlGetTermios(int(file.Fd()), unix.TCGETS); err == nil {
			//log.Printf("===========> IoctlGetTermios :%#v", tio)
			return tio, nil
		} else {
			return nil, fmt.Errorf("cannot termios for file: %s ", file.Name())
		}
	} else {
		return nil, fmt.Errorf("not a terminal file: %s", file.Name())
	}
}

func (t *Terminal) GetTermiosFrom(fds map[int]string) (*unix.Termios, error) {
	for fd, path := range fds {
		// 只关注[标准输入/标准输出/标准错误]的terminal state
		if fd < 3 {
			if tio, err := t.GetFileTermios(path); err == nil {
				return tio, nil
			} else {
				log.Printf("GetTermios: fd: %d , err: %v", fd, err)
			}
		}
	}
	return nil, fmt.Errorf("get std termios failed")
}

func (t *Terminal) ForceInit() error {
	if tio, err := t.GetTermios(os.Stdin); err == nil {
		return t.SetTermios(tio)
	} else {
		return err
	}

}

// Init 初始化(读取目标的tty属性,并赋给当前新申请的pts)
func (t *Terminal) Init(fds map[int]string) error {
	log.Printf("Initializing %s device", t.pts.Name())
	defer func() {
		log.Printf("Device %s has been initialized", t.pts.Name())
	}()
	if tio, err := t.GetTermiosFrom(fds); err == nil {
		return t.SetTermios(tio)
	} else {
		log.Printf("Error: %s, trying to force initialization.", err)
		return t.ForceInit()
	}
}

func (t *Terminal) Ptm() *os.File {
	return t.ptm
}

func (t *Terminal) Pts() *os.File {
	return t.pts
}

func NewTerminal() (*Terminal, error) {
	log.Println("Creating new local pty...")
	ptm, pts, err := pty.Open() //	以后有空试试 termios.Pty()
	if err != nil {
		return nil, err
	}
	defer func() {
		log.Printf("Local pty created, pts: %s", pts.Name())
	}()

	// 用于解决高权限(root)想要访问低权限用户进程, 但是低权限用户进程无法访问高权限(root)创建的pts的问题
	if err := os.Chmod(pts.Name(), 0666); err != nil {
		log.Printf("Chmod(%s, 0666) failed: %s", pts.Name(), err)
	}

	return &Terminal{
		pts: pts,
		ptm: ptm,
	}, nil
}

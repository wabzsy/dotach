package dotach

import (
	"fmt"
	"golang.org/x/term"
	"os"
	"path/filepath"
	"strconv"
)

// 大部分函数改编自 github.com/prometheus/procfs/proc.go 和 github.com/prometheus/procfs/fs.go

const (
	DefaultProcMountPoint = "/proc"
)

type FS string

func NewFS(mountPoint string) (FS, error) {
	info, err := os.Stat(mountPoint)
	if err != nil {
		return "", fmt.Errorf("could not read %q: %w", mountPoint, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("mount point %q is not a directory", mountPoint)
	}

	return FS(mountPoint), nil
}

func (fs FS) Path(p ...string) string {
	return filepath.Join(append([]string{string(fs)}, p...)...)
}

func NewDefaultFS() (FS, error) {
	return NewFS(DefaultProcMountPoint)
}

func (fs FS) Proc(pid int) (Proc, error) {
	if _, err := os.Stat(fs.Path(strconv.Itoa(pid))); err != nil {
		return Proc{}, err
	}
	return Proc{PID: pid, fs: fs}, nil
}

type Proc struct {
	PID int

	fs FS
}

func NewProc(pid int) (Proc, error) {
	fs, err := NewDefaultFS()
	if err != nil {
		return Proc{}, err
	}
	return fs.Proc(pid)
}

// FileDescriptors 获取目标的全部文件描述符(整型版)
func (p Proc) FileDescriptors() ([]int, error) {
	names, err := p.fileDescriptors()
	if err != nil {
		return nil, err
	}

	fds := make([]int, len(names))
	for _, n := range names {
		fd, err := strconv.ParseInt(n, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("could not parse fd %q: %w", n, err)
		}
		fds = append(fds, int(fd))
	}

	return fds, nil
}

// fileDescriptors 获取目标的全部文件描述符(字符串版)
func (p Proc) fileDescriptors() ([]string, error) {
	d, err := os.Open(p.path("fd"))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = d.Close()
	}()

	names, err := d.Readdirnames(-1)
	if err != nil {
		return nil, fmt.Errorf("could not read %q: %w", d.Name(), err)
	}

	return names, nil
}

func (p Proc) path(pa ...string) string {
	return p.fs.Path(append([]string{strconv.Itoa(p.PID)}, pa...)...)
}

// FileDescriptorTargets 获取 /proc/目标PID/fd/ 下的所有条目,并解析link地址
func (p Proc) FileDescriptorTargets() (map[int]string, error) {
	fds, err := p.FileDescriptors()
	if err != nil {
		return nil, err
	}

	targets := make(map[int]string)

	for _, fd := range fds {
		target, err := os.Readlink(p.path("fd", strconv.Itoa(fd)))
		if err == nil {
			targets[fd] = target
		}
	}

	return targets, nil
}

// FileDescriptorAvailable 获取可用的文件描述符(指的是存在的,可用的,不包含已经删除的和socket之类的)
func (p Proc) FileDescriptorAvailable() (map[int]string, error) {
	fds, err := p.FileDescriptorTargets()
	if err != nil {
		return nil, err
	}

	targets := make(map[int]string)

	for fd, path := range fds {
		if _, err := os.Stat(path); err == nil {
			targets[fd] = path
		}
	}

	return targets, nil
}

// IsTerminal 判断目标文件是不是terminal
func IsTerminal(path string) (bool, error) {
	fd, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = fd.Close()
	}()

	return term.IsTerminal(int(fd.Fd())), nil

}

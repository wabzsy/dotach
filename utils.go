package dotach

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"runtime/debug"
)

const (
	PASSWORD = "dotach666"
)

var (
	DebugMode = true
	PauseMode = true
	StackMode = true
)

func PrintStack() {
	stack := debug.Stack()
	stackLines := bytes.Split(stack, []byte("\n"))
	stackLines = stackLines[7:]
	for _, line := range stackLines {
		log.Println(string(line))
	}
}

func Debug(v ...interface{}) {
	if !DebugMode {
		return
	}
	if StackMode {
		log.Println("----------------------[DEBUG]----------------------")
		PrintStack()
	}
	for _, p := range v {
		if err, ok := p.(error); ok {
			log.Printf("Error: %s", err)
		} else {
			log.Printf("Debug: %#v", p)
		}
	}
	if PauseMode {
		log.Println("press ENTER to continue...")
		_, _ = fmt.Scanln()
		log.Println("--------------------[CONTINUED]--------------------")
	}
}

// MagicCopy 改造自 'io.Copy(dst io.Writer, src io.Reader)...'
func MagicCopy(dst io.Writer, src io.Reader) (written int64, err error) {
	return MagicCopyBuffer(dst, src, nil)
}
func MagicCopyBuffer(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {

	//// If the reader has a WriteTo method, use it to do the copy.
	//// Avoids an allocation and a copy.
	//if wt, ok := src.(io.WriterTo); ok {
	//	return wt.WriteTo(dst)
	//}
	//// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	//if rt, ok := dst.(io.ReaderFrom); ok {
	//	return rt.ReadFrom(src)
	//}
	if buf == nil {
		size := 32 * 1024
		if l, ok := src.(*io.LimitedReader); ok && int64(size) > l.N {
			if l.N < 1 {
				size = 1
			} else {
				size = int(l.N)
			}
		}
		buf = make([]byte, size)
	}

	i := 0
	magic := false

	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			// 检查是否输入了3次CTRL+X或粘贴了魔术字符串
			if nr == len(PASSWORD) && string(buf[0:nr]) == PASSWORD {
				magic = true
			} else {
				//if nr == 1 && buf[0] == PASSWORD[i] {
				//	i++
				//	if i == len(PASSWORD) {
				//		magic = true
				//	}
				//} else {
				//	i = 0
				//}
				// CTRL+X = 0x18
				if nr == 1 && buf[0] == 0x18 {
					i++
					if i == 3 {
						magic = true
					}
				} else {
					i = 0
				}
			}

			if magic {
				fmt.Println("\r")
				log.Println("magic string detected\r")
				return written, nil
			}

			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}
			written += int64(nw)
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}
	return written, err
}

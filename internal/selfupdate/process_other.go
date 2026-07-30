//go:build !windows

package selfupdate

import "syscall"

func windowsHiddenProcessAttributes() *syscall.SysProcAttr {
	return nil
}

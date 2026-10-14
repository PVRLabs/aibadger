//go:build windows

package codemap

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Use one native rename operation rather than MoveFileEx, which does not provide
// POSIX atomic replacement semantics. A relative target refers to the source's
// parent directory. There is no cross-volume copy/delete or delete-first fallback.
// https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntifs/ns-ntifs-_file_rename_information
func replaceReport(source, destination string) error {
	if filepath.Dir(source) != filepath.Dir(destination) {
		return fmt.Errorf("report replacement requires the same directory")
	}
	// Keep long absolute paths usable with the native CreateFile entry point.
	if !strings.HasPrefix(source, `\\?\`) {
		if strings.HasPrefix(source, `\\`) {
			source = `\\?\UNC\` + source[2:]
		} else {
			source = `\\?\` + source
		}
	}
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(from, windows.DELETE|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	name, err := windows.UTF16FromString(filepath.Base(destination))
	if err != nil {
		return err
	}
	name = name[:len(name)-1]
	// Native layout follows the x/sys/windows NtSetInformationFile regression.
	type renameInfo struct {
		ReplaceIfExists uint32
		RootDirectory   windows.Handle
		FileNameLength  uint32
		FileName        [1]uint16
	}
	var layout renameInfo
	offset := int(unsafe.Offsetof(layout.FileName))
	buffer := make([]byte, offset+len(name)*2)
	info := (*renameInfo)(unsafe.Pointer(&buffer[0]))
	info.ReplaceIfExists = windows.FILE_RENAME_REPLACE_IF_EXISTS
	info.FileNameLength = uint32(len(name) * 2)
	copy(unsafe.Slice(&info.FileName[0], len(name)), name)
	var status windows.IO_STATUS_BLOCK
	return windows.NtSetInformationFile(handle, &status, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation)
}

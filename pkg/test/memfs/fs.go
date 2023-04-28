package memfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/afero"
)

type ModeAwareMemMapFs struct {
	Fs afero.Fs
}

func NewModeAwareMemFs() *ModeAwareMemMapFs {
	return &ModeAwareMemMapFs{
		Fs: afero.NewMemMapFs(),
	}
}

func (m *ModeAwareMemMapFs) canRead(f fs.FileInfo) error {
	perm := f.Mode().Perm()
	if perm&0444 != 0 {
		return nil
	}
	return &os.PathError{Op: "read", Path: f.Name(), Err: syscall.EACCES}
}

func (m *ModeAwareMemMapFs) canWrite(f fs.FileInfo) error {
	perm := f.Mode().Perm()
	if perm&0222 != 0 {
		return nil
	}
	return &os.PathError{Op: "write", Path: f.Name(), Err: syscall.EACCES}
}

func (m *ModeAwareMemMapFs) canExec(f fs.FileInfo) error {
	perm := f.Mode().Perm()
	if perm&0111 != 0 {
		return nil
	}
	return &os.PathError{Op: "exec", Path: f.Name(), Err: syscall.EACCES}
}

// Create creates a file in the filesystem, returning the file and an
// error, if any happens.
func (m *ModeAwareMemMapFs) Create(name string) (afero.File, error) {
	dir := filepath.Dir(name)
	f, err := m.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !f.IsDir() {
		return nil, &os.PathError{Op: "create", Path: name, Err: syscall.ENOTDIR}
	}
	if err := m.canWrite(f); err != nil {
		return nil, err
	}
	if err := m.canExec(f); err != nil {
		return nil, err
	}
	return m.Fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

// Mkdir creates a directory in the filesystem, return an error if any
// happens.
func (m *ModeAwareMemMapFs) Mkdir(name string, perm os.FileMode) error {
	dir := filepath.Dir(name)
	f, err := m.Stat(dir)
	if err != nil {
		return err
	}
	if err := m.canWrite(f); err != nil {
		return err
	}
	if err := m.canExec(f); err != nil {
		return err
	}
	return m.Fs.Mkdir(name, perm)
}

// MkdirAll creates a directory path and all parents that does not exist
// yet.
func (m *ModeAwareMemMapFs) MkdirAll(path string, perm os.FileMode) error {
	// implementation from os.MkdirAll

	// Fast path: if we can tell whether path is a directory or file, stop with success or error.
	dir, err := m.Stat(path)
	if err == nil {
		if dir.IsDir() {
			return nil
		}
		return &os.PathError{Op: "mkdir", Path: path, Err: syscall.ENOTDIR}
	}

	// Slow path: make sure parent exists and then call Mkdir for path.
	i := len(path)
	for i > 0 && os.IsPathSeparator(path[i-1]) { // Skip trailing path separator.
		i--
	}

	j := i
	for j > 0 && !os.IsPathSeparator(path[j-1]) { // Scan backward over element.
		j--
	}

	if j > 1 {
		// Create parent.
		err = m.MkdirAll(path[:j-1], perm)
		if err != nil {
			return err
		}
	}

	// Parent now exists; invoke Mkdir and use its result.
	err = m.Mkdir(path, perm)
	if err != nil && !os.IsExist(err) {
		// Handle arguments like "foo/." by
		// double-checking that directory doesn't exist.
		dir, err1 := m.Stat(path)
		if err1 == nil && dir.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

// Open opens a file, returning it or an error, if any happens.
func (m *ModeAwareMemMapFs) Open(name string) (afero.File, error) {
	f, err := m.Fs.Open(name)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if err := m.canRead(fi); err != nil {
		return nil, err
	}
	return f, nil
}

// OpenFile opens a file using the given flags and the given mode.
func (m *ModeAwareMemMapFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	var fileExists bool
	if _, err := m.Fs.Stat(name); err == nil {
		fileExists = true
	}
	if !fileExists && perm == 0 {
		runtime.Breakpoint()
	}
	f, err := m.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if perm&fs.FileMode(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		if !fileExists {
			// check write permission on parent directory
			dir := filepath.Dir(name)
			f, err := m.Stat(dir)
			if err != nil {
				return nil, err
			}
			if err := m.canWrite(f); err != nil {
				return nil, err
			}
		}

		if err := m.canWrite(fi); err != nil {
			return nil, err
		}
	}
	if perm&fs.FileMode(os.O_RDONLY) != 0 {
		if err := m.canRead(fi); err != nil {
			return nil, err
		}
	}
	return f, nil
}

// Remove removes a file identified by name, returning an error, if any
// happens.
func (m *ModeAwareMemMapFs) Remove(name string) error {
	dir := filepath.Dir(name)
	f, err := m.Stat(dir)
	if err != nil {
		return err
	}
	if err := m.canWrite(f); err != nil {
		return err
	}
	if err := m.canExec(f); err != nil {
		return err
	}

	return m.Fs.Remove(name)
}

// RemoveAll removes a directory path and any children it contains. It
// does not fail if the path does not exist (return nil).
func (m *ModeAwareMemMapFs) RemoveAll(path string) error {
	// not correct but good enough for testing
	dir := filepath.Dir(path)
	f, err := m.Stat(dir)
	if err != nil {
		return err
	}
	if err := m.canWrite(f); err != nil {
		return err
	}
	if err := m.canExec(f); err != nil {
		return err
	}
	return m.Fs.RemoveAll(path)
}

// Rename renames a file.
func (m *ModeAwareMemMapFs) Rename(oldname string, newname string) error {
	newDir := filepath.Dir(newname)
	newF, err := m.Stat(newDir)
	if err != nil {
		return err
	}
	oldF, err := m.Stat(oldname)
	if err != nil {
		return err
	}
	if err := m.canRead(oldF); err != nil {
		return err
	}
	if err := m.canWrite(newF); err != nil {
		return err
	}
	return m.Fs.Rename(oldname, newname)
}

func (m *ModeAwareMemMapFs) Stat(name string) (os.FileInfo, error) {
	var dir fs.FileInfo
	// ensure we have exec permission on all parents
	if name != "/" {
		var err error
		dir, err = m.Stat(filepath.Dir(name))
		if err != nil {
			return nil, err
		}
	} else {
		dir, _ = m.Fs.Stat(name)
	}
	info, err := m.Fs.Stat(name)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		if err := m.canExec(info); err != nil {
			return nil, err
		}
	}
	if err := m.canExec(dir); err != nil {
		return nil, err
	}
	return info, nil
}

func (m *ModeAwareMemMapFs) Chmod(name string, mode os.FileMode) error {
	_, err := m.Stat(filepath.Dir(name))
	if err != nil {
		return err
	}
	return m.Fs.Chmod(name, mode)
}

func (m *ModeAwareMemMapFs) Chown(name string, uid, gid int) error {
	_, err := m.Stat(filepath.Dir(name))
	if err != nil {
		return err
	}
	return m.Fs.Chown(name, uid, gid)
}

func (m *ModeAwareMemMapFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	_, err := m.Stat(filepath.Dir(name))
	if err != nil {
		return err
	}
	return m.Fs.Chtimes(name, atime, mtime)
}

func (m *ModeAwareMemMapFs) Name() string {
	return "ModeAwareMemMapFs"
}

// A filesystem wrapper whose implementation of Rename will return EXDEV if the
// source and destination paths do not share the same first path component.
// This is to simulate cross-device rename errors.
type CrossDeviceTestFs struct {
	afero.Fs
}

func (c *CrossDeviceTestFs) Rename(oldpath, newpath string) error {
	oldpath = filepath.Clean(oldpath)
	newpath = filepath.Clean(newpath)
	if !filepath.IsAbs(oldpath) {
		var err error
		oldpath, err = filepath.Abs(oldpath)
		if err != nil {
			panic("test error: could not get absolute path for oldpath")
		}
	}
	if !filepath.IsAbs(newpath) {
		var err error
		newpath, err = filepath.Abs(newpath)
		if err != nil {
			panic("test error: could not get absolute path for newpath")
		}
	}

	// check if the first path component is the same, starting from the root
	oldpathComponents := strings.Split(oldpath, string(filepath.Separator))
	newpathComponents := strings.Split(newpath, string(filepath.Separator))
	if len(oldpathComponents) < 2 || len(newpathComponents) < 2 {
		panic("test error: Rename called with invalid paths")

	}
	if oldpathComponents[1] == newpathComponents[1] {
		return c.Fs.Rename(oldpath, newpath)
	}
	return &os.LinkError{
		Op:  "rename",
		Old: oldpath,
		New: newpath,
		Err: syscall.EXDEV,
	}
}

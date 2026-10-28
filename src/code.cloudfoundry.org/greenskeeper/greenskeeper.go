package greenskeeper

import (
	"fmt"
	"os"
	"path"
	"strings"
	"syscall"
)

const defaultDirectoryMode = os.FileMode(0701)

type Directory struct {
	Path string
	Mode *os.FileMode
	UID  int
	GID  int

	mkdirAll func(string, os.FileMode) error
	chown    func(string, int, int) error
	chmod    func(string, os.FileMode) error
}

type DirectoryBuilder struct {
	directory Directory
}

func NewDirectoryBuilder(path string) DirectoryBuilder {
	return DirectoryBuilder{directory: Directory{
		Path: path,
		UID:  -1,
		GID:  -1,

		mkdirAll: os.MkdirAll,
		chown:    chownNoFollow,
		chmod:    chmodNoFollow,
	}}
}

func (b DirectoryBuilder) Build() Directory {
	return b.directory
}

func (b DirectoryBuilder) UID(uid int) DirectoryBuilder {
	b.directory.UID = uid
	return b
}

func (b DirectoryBuilder) GID(gid int) DirectoryBuilder {
	b.directory.GID = gid
	return b
}

func (b DirectoryBuilder) Mode(mode os.FileMode) DirectoryBuilder {
	b.directory.Mode = &mode
	return b
}

// openDirNoFollow opens path as a directory without following a symlink in the
// final path component. greenskeeper runs as root on paths that live inside a
// directory writable by a less privileged user, so chmod/chown must never be
// applied through a symlink that user planted.
func openDirNoFollow(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}

	return os.NewFile(uintptr(fd), path), nil
}

func chmodNoFollow(path string, mode os.FileMode) error {
	f, err := openDirNoFollow(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return f.Chmod(mode)
}

func chownNoFollow(path string, uid, gid int) error {
	f, err := openDirNoFollow(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return f.Chown(uid, gid)
}

func CreateDirectories(directories ...Directory) error {
	for _, directory := range directories {
		if err := directory.Create(); err != nil {
			return err
		}
	}

	return nil
}

func (d Directory) Create() error {
	if err := d.mkdirAll(d.Path, defaultDirectoryMode); err != nil {
		return err
	}

	if d.Mode != nil {
		if err := d.chmod(d.Path, *d.Mode); err != nil {
			return err
		}
	}

	if d.GID > -1 {
		return d.chown(d.Path, d.UID, d.GID)
	}

	return nil
}

func CheckExistingGdnProcess(pidFilePath string) error {
	return checkExistingGdnProcess(pidFilePath, os.Remove)
}

func checkExistingGdnProcess(pidFilePath string, remove func(string) error) error {
	contents, err := os.ReadFile(pidFilePath)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	pid := strings.TrimSpace(string(contents))
	if isRunning(pid) {
		return fmt.Errorf("garden is already running (pid: %s)", pid)
	}

	fmt.Println("Removing stale pidfile...")
	return remove(pidFilePath)
}

func isRunning(pid string) bool {
	if _, err := os.Stat(path.Join("/proc", pid)); pid != "" && err == nil {
		return true
	}
	return false
}

func newFileMode(mode os.FileMode) *os.FileMode {
	fileMode := mode
	return &fileMode
}

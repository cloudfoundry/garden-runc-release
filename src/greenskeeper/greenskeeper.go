package greenskeeper

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"strconv"
	"strings"
)

const defaultDirectoryMode = os.FileMode(0777)

type Directory struct {
	Path  string
	Mode  *os.FileMode
	User  string
	Group string

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

		mkdirAll: os.MkdirAll,
		chown:    os.Chown,
		chmod:    os.Chmod,
	}}
}

func (b DirectoryBuilder) Build() Directory {
	return b.directory
}

func (b DirectoryBuilder) User(user string) DirectoryBuilder {
	b.directory.User = user
	return b
}

func (b DirectoryBuilder) Group(group string) DirectoryBuilder {
	b.directory.Group = group
	return b
}

func (b DirectoryBuilder) Mode(mode os.FileMode) DirectoryBuilder {
	b.directory.Mode = &mode
	return b
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

	user, err := d.getUID()
	if err != nil {
		return err
	}

	group, err := d.getGID()
	if err != nil {
		return err
	}

	return d.chown(d.Path, user, group)
}

func (d Directory) getMode() {
}

func (d Directory) getUID() (int, error) {
	if d.User == "" {
		currentUser, err := user.Current()
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(currentUser.Uid)
	}

	directoryUser, err := user.Lookup(d.User)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(directoryUser.Uid)
}

func (d Directory) getGID() (int, error) {
	if d.Group == "" {
		currentUser, err := user.Current()
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(currentUser.Gid)
	}

	directoryUser, err := user.Lookup(d.User)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(directoryUser.Gid)
}

func CheckExistingGdnProcess(pidFilePath string) error {
	return checkExistingGdnProcess(pidFilePath, os.Remove)
}

func checkExistingGdnProcess(pidFilePath string, remove func(string) error) error {
	contents, err := ioutil.ReadFile(pidFilePath)
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

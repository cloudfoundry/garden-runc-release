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

type Directory struct {
	Path  string
	Mode  os.FileMode
	User  string
	Group string

	mkdirAll func(string, os.FileMode) error
}

func NewDirectory(path string, mode os.FileMode, user, group string) Directory {
	return Directory{
		Path:     path,
		Mode:     mode,
		User:     user,
		Group:    group,
		mkdirAll: os.MkdirAll,
	}
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

func SetupDirectories(directories ...Directory) error {
	for _, directory := range directories {
		if err := directory.Setup(); err != nil {
			return err
		}
	}

	return nil
}

func (d Directory) Setup() error {
	if err := d.mkdirAll(d.Path, d.Mode); err != nil {
		return err
	}

	uid, err := d.GetUID()
	if err != nil {
		return err
	}

	gid, err := d.GetGID()
	if err != nil {
		return err
	}

	os.Chown(d.Path, uid, gid)
	return nil
}

func (d Directory) GetUID() (int, error) {
	directoryUser, err := user.Lookup(d.User)
	if err != nil {
		return -1, err
	}

	return strconv.Atoi(directoryUser.Uid)
}

func (d Directory) GetGID() (int, error) {
	directoryUser, err := user.Lookup(d.User)
	if err != nil {
		return -1, err
	}

	return strconv.Atoi(directoryUser.Gid)
}

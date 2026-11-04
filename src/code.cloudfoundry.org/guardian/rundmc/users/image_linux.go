package users

import (
	"errors"
	"fmt"
	"io"
	"os"

	securejoin "github.com/cyphar/filepath-securejoin/pathrs-lite"
	"github.com/moby/sys/user"
)

func fileReader(file *os.File) io.Reader {
	if file == nil {
		return nil
	}
	return file
}

func openImageIdentity(root, path string) (*os.File, error) {
	handle, err := securejoin.OpenInRoot(root, path)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("image identity %s is not a regular file", path)
	}
	// OpenInRoot returns an O_PATH descriptor. Reopen the pinned inode rather
	// than its pathname, retaining confinement without attempting O_PATH reads.
	return securejoin.Reopen(handle, os.O_RDONLY)
}

// LookupImageUser reads identity files inside a prepared image root. Absolute
// image symlinks must not resolve against Guardian's host filesystem.
func LookupImageUser(root, name string) (*ExecUser, error) {
	passwd, err := openImageIdentity(root, "/etc/passwd")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if passwd != nil {
		defer passwd.Close()
	}
	group, err := openImageIdentity(root, "/etc/group")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if group != nil {
		defer group.Close()
	}
	if passwd == nil && name == "root" {
		return &ExecUser{Uid: DefaultUID, Gid: DefaultGID, Home: DefaultHome}, nil
	}
	// Avoid a typed nil *os.File as an io.Reader when an image omits a file.
	var passwdReader, groupReader = fileReader(passwd), fileReader(group)
	resolved, err := user.GetExecUser(name, &user.ExecUser{Uid: DefaultUID, Gid: DefaultGID, Home: DefaultHome}, passwdReader, groupReader)
	if err != nil {
		return nil, err
	}
	return &ExecUser{Uid: resolved.Uid, Gid: resolved.Gid, Home: resolved.Home, Sgids: resolved.Sgids}, nil
}

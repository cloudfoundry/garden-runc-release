package users_test

import (
	"os"
	"path/filepath"
	"testing"

	"code.cloudfoundry.org/guardian/rundmc/users"
)

func TestLookupImageUser(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "etc"), 0755); err != nil {
		t.Fatal(err)
	}
	// Absolute symlinks refer to the image root, never the host's /etc/passwd.
	if err := os.WriteFile(filepath.Join(root, "image-passwd"), []byte("childuser:x:1234:4321:child:/child-home:/bin/sh\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/image-passwd", filepath.Join(root, "etc/passwd")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "etc/group"), []byte("childgroup:x:4321:\nextra:x:5678:childuser\n"), 0600); err != nil {
		t.Fatal(err)
	}
	resolved, err := users.LookupImageUser(root, "childuser")
	if err != nil || resolved.Uid != 1234 || resolved.Gid != 4321 || resolved.Home != "/child-home" || len(resolved.Sgids) != 1 || resolved.Sgids[0] != 5678 {
		t.Fatalf("image identity = %+v, %v", resolved, err)
	}
	if _, err := users.LookupImageUser(root, "root"); err == nil {
		t.Fatal("must not resolve root from the host when image passwd lacks it")
	}
}

func TestLookupImageUserMissingFiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"root", "1234:4321"} {
		resolved, err := users.LookupImageUser(root, name)
		if err != nil || resolved == nil {
			t.Fatalf("%s = %+v, %v", name, resolved, err)
		}
	}
	if _, err := users.LookupImageUser(root, "childuser"); err == nil {
		t.Fatal("missing named image user was accepted")
	}
}

package nerd_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProcStat(t *testing.T) {
	valid := "123 (comm with ) spaces) S 456 " + strings.Repeat("0 ", 17) + "98765 0"
	identity, err := parseProcStat(valid)
	if err != nil || identity.PID != 123 || identity.Parent != 456 || identity.Start != 98765 {
		t.Fatalf("identity=%+v err=%v", identity, err)
	}
	for _, contents := range []string{"", "123 missing", "123 (short) S 456", strings.Replace(valid, "98765", "oops", 1), strings.Replace(valid, "S 456", "S bad", 1), strings.Replace(valid, "98765", "0", 1)} {
		if _, err := parseProcStat(contents); err == nil {
			t.Errorf("accepted malformed stat %q", contents)
		}
	}
}

func TestSelectTaskProcesses(t *testing.T) {
	bundle := "/run/containerd/ns/task-id"
	fixture := func() map[int]procIdentity {
		return map[int]procIdentity{
			10: {PID: 10, Parent: 1, Start: 100, State: "S", Executable: "/bin/containerd-shim-runsc-v1", Args: []string{"containerd-shim-runsc-v1", "-id", "task-id"}},
			11: {PID: 11, Parent: 10, Start: 101, State: "S", Executable: "/bin/gvisor_sentry", Args: []string{"runsc-sandbox", "boot", "task-id"}},
			12: {PID: 12, Parent: 1, Start: 102, State: "S", Executable: "/bin/runsc", Args: []string{"runsc-gofer", "gofer", "--bundle", bundle}},
			13: {PID: 13, Parent: 12, Start: 103, State: "S", Executable: "/bin/runsc", Args: []string{"worker"}},
			14: {PID: 14, Parent: 1, Start: 104, State: "S", Executable: "/bin/runsc", Args: []string{"runsc-gofer", "--bundle", bundle + "-other"}},
			2:  {PID: 2, Parent: 0, Start: 90, State: "S"},
		}
	}
	all := fixture()
	identities, err := selectTaskProcesses(all, 11, "task-id", bundle)
	if err != nil || len(identities) != 4 {
		t.Fatalf("identities=%+v err=%v", identities, err)
	}
	for _, identity := range identities {
		if identity.PID == 14 {
			t.Fatal("selected unrelated gofer")
		}
	}
	all = fixture()
	all[15] = procIdentity{PID: 15, Parent: 11, Start: 105, State: "S", Executable: all[11].Executable}
	identities, err = selectTaskProcesses(all, 11, "task-id", bundle)
	if err != nil || len(identities) != 5 {
		t.Fatalf("sentry stub identities=%+v err=%v", identities, err)
	}
	for _, mutate := range []func(map[int]procIdentity){
		func(all map[int]procIdentity) { stub := all[15]; stub.Executable = "/bin/unrelated"; all[15] = stub },
		func(all map[int]procIdentity) { stub := all[15]; stub.Parent = 10; all[15] = stub },
		func(all map[int]procIdentity) { stub := all[15]; stub.Start = 0; all[15] = stub },
		func(all map[int]procIdentity) { stub := all[15]; stub.State = "Z"; all[15] = stub },
		func(all map[int]procIdentity) { task := all[11]; task.Args = nil; all[11] = task },
	} {
		negative := make(map[int]procIdentity, len(all))
		for pid, identity := range all {
			negative[pid] = identity
		}
		mutate(negative)
		if _, err := selectTaskProcesses(negative, 11, "task-id", bundle); err == nil {
			t.Fatal("accepted unproven argv-free process")
		}
	}
	for _, mutate := range []func(map[int]procIdentity){
		func(all map[int]procIdentity) { delete(all, 12) },
		func(all map[int]procIdentity) {
			shim := all[10]
			shim.Args = []string{"shim", "-id", "other"}
			all[10] = shim
		},
		func(all map[int]procIdentity) {
			shim := all[10]
			shim.Executable = "/bin/unrelated-containerd-shim"
			all[10] = shim
		},
		func(all map[int]procIdentity) { task := all[11]; task.Start = 0; all[11] = task },
	} {
		all := fixture()
		mutate(all)
		if _, err := selectTaskProcesses(all, 11, "task-id", bundle); err == nil {
			t.Fatal("accepted missing identity proof")
		}
	}
}

func TestReadProcIdentityWithoutArgv(t *testing.T) {
	root := t.TempDir()
	processRoot := filepath.Join(root, "123")
	if err := os.Mkdir(processRoot, 0700); err != nil {
		t.Fatal(err)
	}
	stat := "123 (stub) S 456 " + strings.Repeat("0 ", 17) + "98765 0"
	for name, contents := range map[string]string{"stat": stat, "cmdline": ""} {
		if err := os.WriteFile(filepath.Join(processRoot, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	identity, err := readProcIdentityFrom(root, 123)
	if err != nil || identity.Start != 98765 || identity.Executable != "" || len(identity.Args) != 0 {
		t.Fatalf("kernel thread identity=%+v err=%v", identity, err)
	}
	if err := os.Symlink("/bin/gvisor_sentry", filepath.Join(processRoot, "exe")); err != nil {
		t.Fatal(err)
	}
	identity, err = readProcIdentityFrom(root, 123)
	if err != nil || identity.Start != 98765 || identity.Executable != "/bin/gvisor_sentry" || len(identity.Args) != 0 {
		t.Fatalf("stub identity=%+v err=%v", identity, err)
	}
}

func TestParseProcCmdline(t *testing.T) {
	args, err := parseProcCmdline("runsc-gofer\x00--bundle\x00/run/task with spaces\x00")
	if err != nil || len(args) != 3 || args[2] != "/run/task with spaces" {
		t.Fatalf("args=%q err=%v", args, err)
	}
	for _, contents := range []string{"", "runsc", "\x00--bundle\x00"} {
		if _, err := parseProcCmdline(contents); err == nil {
			t.Errorf("accepted malformed cmdline %q", contents)
		}
	}
	if !procFlag([]string{"gofer", "--bundle=/run/task"}, "--bundle", "/run/task") || procFlag([]string{"gofer", "--bundle=/run/task-other"}, "--bundle", "/run/task") {
		t.Fatal("bundle flag comparison is not exact")
	}
}

func TestTaskProcessesGone(t *testing.T) {
	identities := []procIdentity{{PID: 11, Start: 101}, {PID: 12, Start: 102, Executable: "/bin/runsc", Args: []string{"runsc-gofer"}}}
	for _, state := range []string{"S", "Z"} {
		err := taskProcessesGone(identities, func(pid int) (procIdentity, error) {
			if pid == 11 {
				return procIdentity{}, os.ErrNotExist
			}
			return procIdentity{PID: 12, Parent: 1, Start: 102, State: state}, nil
		})
		if err == nil || !strings.Contains(err.Error(), "runsc-gofer") {
			t.Fatalf("surviving reparented gofer not diagnosed: %v", err)
		}
	}
	if err := taskProcessesGone(identities, func(pid int) (procIdentity, error) { return procIdentity{PID: pid, Start: 999}, nil }); err != nil {
		t.Fatal(err)
	}
	if err := taskProcessesGone(identities, func(int) (procIdentity, error) { return procIdentity{}, os.ErrNotExist }); err != nil {
		t.Fatal(err)
	}
	if err := taskProcessesGone(identities, func(int) (procIdentity, error) { return procIdentity{}, fmt.Errorf("permission denied") }); err == nil {
		t.Fatal("ignored proc read error")
	}
}

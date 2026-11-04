package nerd_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type procIdentity struct {
	PID        int
	Parent     int
	Start      uint64
	State      string
	Executable string
	Args       []string
}

func parseProcStat(contents string) (procIdentity, error) {
	open, close := strings.IndexByte(contents, '('), strings.LastIndexByte(contents, ')')
	if open < 1 || close <= open {
		return procIdentity{}, fmt.Errorf("invalid proc stat: missing comm delimiters: %q", contents)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(contents[:open]))
	if err != nil || pid <= 0 {
		return procIdentity{}, fmt.Errorf("invalid proc stat PID: %q", contents)
	}
	fields := strings.Fields(contents[close+1:])
	if len(fields) < 20 {
		return procIdentity{}, fmt.Errorf("proc stat PID %d: missing field 22", pid)
	}
	parent, err := strconv.Atoi(fields[1])
	if err != nil || parent < 0 {
		return procIdentity{}, fmt.Errorf("proc stat PID %d: invalid PPID %q", pid, fields[1])
	}
	start, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || start == 0 {
		return procIdentity{}, fmt.Errorf("proc stat PID %d: invalid starttime %q", pid, fields[19])
	}
	if len(fields[0]) != 1 || !strings.Contains("RSDZTtXxKWPI", fields[0]) {
		return procIdentity{}, fmt.Errorf("proc stat PID %d: invalid state %q", pid, fields[0])
	}
	return procIdentity{PID: pid, Parent: parent, Start: start, State: fields[0]}, nil
}

func procStat(pid int) (procIdentity, error) {
	return procStatFrom("/proc", pid)
}

func procStatFrom(procRoot string, pid int) (procIdentity, error) {
	contents, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "stat"))
	if err != nil {
		return procIdentity{}, err
	}
	identity, err := parseProcStat(string(contents))
	if err == nil && identity.PID != pid {
		return procIdentity{}, fmt.Errorf("proc PID mismatch: requested %d, got %d", pid, identity.PID)
	}
	return identity, err
}

func readProcIdentity(pid int) (procIdentity, error) {
	return readProcIdentityFrom("/proc", pid)
}

func readProcIdentityFrom(procRoot string, pid int) (procIdentity, error) {
	identity, err := procStatFrom(procRoot, pid)
	if err != nil {
		return identity, err
	}
	root := filepath.Join(procRoot, strconv.Itoa(pid))
	cmdline, err := os.ReadFile(filepath.Join(root, "cmdline"))
	if err != nil {
		return identity, err
	}
	if len(cmdline) != 0 {
		identity.Args, err = parseProcCmdline(string(cmdline))
		if err != nil {
			return identity, fmt.Errorf("PID %d: %w", pid, err)
		}
	}
	identity.Executable, err = os.Readlink(filepath.Join(root, "exe"))
	if err != nil && !(len(cmdline) == 0 && os.IsNotExist(err)) {
		return identity, err
	}
	verified, err := procStatFrom(procRoot, pid)
	if err != nil {
		return identity, err
	}
	if verified.Start != identity.Start || verified.Parent != identity.Parent {
		return identity, fmt.Errorf("PID %d reused or reparented during capture", pid)
	}
	return identity, nil
}

func parseProcCmdline(contents string) ([]string, error) {
	if len(contents) == 0 || contents[len(contents)-1] != 0 {
		return nil, fmt.Errorf("empty or malformed proc cmdline: %q", contents)
	}
	args := strings.Split(contents[:len(contents)-1], "\x00")
	if args[0] == "" {
		return nil, fmt.Errorf("proc cmdline has empty argv[0]")
	}
	return args, nil
}

func procFlag(args []string, flag, value string) bool {
	for index, arg := range args {
		if arg == flag+"="+value || (arg == flag && index+1 < len(args) && args[index+1] == value) {
			return true
		}
	}
	return false
}

func runtimeExecutable(executable string) bool {
	switch filepath.Base(strings.TrimSuffix(executable, " (deleted)")) {
	case "runsc", "gvisor_sentry", "containerd-shim-runsc-v1":
		return true
	}
	return false
}

func selectTaskProcesses(all map[int]procIdentity, taskPID int, taskID, bundle string) ([]procIdentity, error) {
	task, ok := all[taskPID]
	if !ok {
		return nil, fmt.Errorf("task %s PID %d missing from proc snapshot", taskID, taskPID)
	}
	shim, ok := all[task.Parent]
	if !ok || filepath.Base(shim.Executable) != "containerd-shim-runsc-v1" || !procFlag(shim.Args, "-id", taskID) {
		return nil, fmt.Errorf("task %s: no dedicated runsc shim parent proof: task=%+v parent=%+v", taskID, task, shim)
	}
	if !runtimeExecutable(task.Executable) || len(task.Args) == 0 || filepath.Base(bundle) != taskID {
		return nil, fmt.Errorf("task %s: unproven runtime executable or bundle: task=%+v bundle=%q", taskID, task, bundle)
	}
	selected := map[int]bool{shim.PID: true}
	for changed := true; changed; {
		changed = false
		for pid, identity := range all {
			associated := procFlag(identity.Args, "--bundle", bundle) && runtimeExecutable(identity.Executable)
			if !selected[pid] && (selected[identity.Parent] || associated) {
				selected[pid] = true
				changed = true
			}
		}
	}
	var identities []procIdentity
	gofer := false
	for pid := range selected {
		identity := all[pid]
		stub := false
		if len(identity.Args) == 0 && identity.Executable == task.Executable {
			visited := map[int]bool{pid: true}
			for parent := identity.Parent; parent != 0 && !visited[parent]; parent = all[parent].Parent {
				if parent == taskPID {
					stub = true
					break
				}
				visited[parent] = true
			}
		}
		if identity.Start == 0 || identity.Executable == "" || (len(identity.Args) == 0 && !stub) || identity.State == "Z" {
			return nil, fmt.Errorf("task %s: incomplete pre-delete process identity: %+v", taskID, identity)
		}
		if len(identity.Args) != 0 && filepath.Base(identity.Args[0]) == "runsc-gofer" && runtimeExecutable(identity.Executable) && procFlag(identity.Args, "--bundle", bundle) {
			gofer = true
		}
		identities = append(identities, identity)
	}
	if !gofer {
		return nil, fmt.Errorf("task %s: no gofer with exact bundle %q in pre-delete snapshot: %+v", taskID, bundle, identities)
	}
	return identities, nil
}

func captureTaskProcesses(taskPID int, taskID string) ([]procIdentity, error) {
	task, err := procStat(taskPID)
	if err != nil {
		return nil, err
	}
	bundle, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(task.Parent), "cwd"))
	if err != nil {
		return nil, fmt.Errorf("task %s shim bundle: %w", taskID, err)
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	all := map[int]procIdentity{}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		identity, err := readProcIdentity(pid)
		if os.IsNotExist(err) {
			stat, statErr := procStat(pid)
			if os.IsNotExist(statErr) || (statErr == nil && stat.State == "Z") {
				continue
			}
		}
		if err != nil {
			return nil, fmt.Errorf("task %s: cannot capture PID %d: %w", taskID, pid, err)
		}
		all[pid] = identity
	}
	return selectTaskProcesses(all, taskPID, taskID, bundle)
}

func taskProcessesGone(identities []procIdentity, readStat func(int) (procIdentity, error)) error {
	var remaining []string
	for _, identity := range identities {
		current, err := readStat(identity.PID)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("checking captured process %+v: %w", identity, err)
		}
		if current.Start == identity.Start {
			remaining = append(remaining, fmt.Sprintf("captured=%+v current=%+v", identity, current))
		}
	}
	if len(remaining) != 0 {
		return fmt.Errorf("task runtime processes remain (including zombies): %s", strings.Join(remaining, "; "))
	}
	return nil
}

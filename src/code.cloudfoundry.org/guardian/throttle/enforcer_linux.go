package throttle

import (
	"encoding/json"
	"os"
	"path/filepath"

	gardencgroups "code.cloudfoundry.org/guardian/rundmc/cgroups"

	"code.cloudfoundry.org/lager/v3"
	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/utils"
)

const (
	memoryMaxFile     = "memory.max"
	memorySwapMaxFile = "memory.swap.max"
	// unlimitedMemory is the value cgroup v2 uses to represent "no limit" for
	// memory.max / memory.swap.max.
	unlimitedMemory = "max"
)

// memoryLimitFiles lists the cgroup v2 memory-limit files that must follow a
// container as it is moved into the CPU-throttling "bad" cgroup, so that the
// memory limit keeps being enforced while a container is throttled.
var memoryLimitFiles = []string{memoryMaxFile, memorySwapMaxFile}

type CPUCgroupEnforcer struct {
	goodCgroupPath string
	badCgroupPath  string
	cpuSharesFile  string
	runcRoot       string
	namespace      string
}

func NewEnforcer(cpuCgroupPath string, runcRoot string, namespace string) CPUCgroupEnforcer {
	cpuSharesFile := "cpu.shares"
	if cgroups.IsCgroup2UnifiedMode() {
		cpuSharesFile = "cpu.weight"
	}

	return CPUCgroupEnforcer{
		goodCgroupPath: filepath.Join(cpuCgroupPath, gardencgroups.GoodCgroupName),
		badCgroupPath:  filepath.Join(cpuCgroupPath, gardencgroups.BadCgroupName),
		cpuSharesFile:  cpuSharesFile,
		runcRoot:       runcRoot,
		namespace:      namespace,
	}
}

func (c CPUCgroupEnforcer) Punish(logger lager.Logger, handle string) error {
	logger = logger.Session("punish", lager.Data{"handle": handle})
	logger.Info("starting")
	defer logger.Info("finished")

	goodContainerCgroupPath := filepath.Join(c.goodCgroupPath, handle)
	if !exists(logger, goodContainerCgroupPath) {
		logger.Info("good-cgroup-does-not-exist-skip-punish", lager.Data{"handle": handle, "goodContainerCgroupPath": goodContainerCgroupPath})
		return nil
	}

	badContainerCgroupPath := filepath.Join(c.badCgroupPath, handle)

	// On cgroup v2 the bad cgroup is a sibling of the good cgroup in the single
	// unified hierarchy. runc sets memory.max on the container's (good) cgroup,
	// so moving the PIDs into the bad cgroup would drop the memory limit unless
	// we propagate it. The memory limit always lives on the container cgroup
	// (good/<handle>), not on the init sub-cgroup, so copy it from there.
	if err := c.copyMemoryLimits(logger, goodContainerCgroupPath, badContainerCgroupPath); err != nil {
		return err
	}

	// in cgroups v2 containerd garden-init process is added to init cgroup
	goodInitCgroupPath := filepath.Join(goodContainerCgroupPath, gardencgroups.InitCgroupName)
	if exists(logger, goodInitCgroupPath) {
		if err := c.copyShares(goodInitCgroupPath, badContainerCgroupPath); err != nil {
			return err
		}

		if err := c.movePids(goodInitCgroupPath, badContainerCgroupPath); err != nil {
			return err
		}

		return c.updateContainerStateCgroupPath(handle, badContainerCgroupPath)
	}

	if err := c.copyShares(goodContainerCgroupPath, badContainerCgroupPath); err != nil {
		return err
	}

	if err := c.movePids(goodContainerCgroupPath, badContainerCgroupPath); err != nil {
		return err
	}

	return c.updateContainerStateCgroupPath(handle, badContainerCgroupPath)
}

func (c CPUCgroupEnforcer) Release(logger lager.Logger, handle string) error {
	logger = logger.Session("release", lager.Data{"handle": handle})
	logger.Info("starting")
	defer logger.Info("finished")

	badContainerCgroupPath := filepath.Join(c.badCgroupPath, handle)
	if !exists(logger, badContainerCgroupPath) {
		logger.Info("bad-cgroup-does-not-exist-skip-punish", lager.Data{"handle": handle, "badContainerCgroupPath": badContainerCgroupPath})
		return nil
	}

	goodContainerCgroupPath := filepath.Join(c.goodCgroupPath, handle)

	// in cgroups v2 containerd garden-init process is added to init cgroup
	goodInitCgroupPath := filepath.Join(goodContainerCgroupPath, gardencgroups.InitCgroupName)
	if exists(logger, goodInitCgroupPath) {
		if err := c.movePids(badContainerCgroupPath, goodInitCgroupPath); err != nil {
			return err
		}
		if err := c.clearMemoryLimits(logger, badContainerCgroupPath); err != nil {
			return err
		}
		return c.updateContainerStateCgroupPath(handle, goodInitCgroupPath)
	}

	if err := c.movePids(badContainerCgroupPath, goodContainerCgroupPath); err != nil {
		return err
	}
	if err := c.clearMemoryLimits(logger, badContainerCgroupPath); err != nil {
		return err
	}
	return c.updateContainerStateCgroupPath(handle, goodContainerCgroupPath)
}

func (c CPUCgroupEnforcer) movePids(fromCgroup, toCgroup string) error {
	for {
		pids, err := cgroups.GetPids(fromCgroup)
		if err != nil {
			return err
		}

		if len(pids) == 0 {
			return nil
		}

		for _, pid := range pids {
			if err = cgroups.WriteCgroupProc(toCgroup, pid); err != nil {
				return err
			}
		}
	}
}

func (c CPUCgroupEnforcer) copyShares(fromCgroup, toCgroup string) error {
	containerShares, err := os.ReadFile(filepath.Join(fromCgroup, c.cpuSharesFile))
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(toCgroup, c.cpuSharesFile), containerShares, 0644)
}

// copyMemoryLimits propagates the container's cgroup v2 memory limits from the
// good cgroup to the bad cgroup so that a throttled container keeps being
// OOM-killed when it exceeds its memory limit. It is a no-op on cgroup v1,
// where memory lives in a separate hierarchy that the CPU-throttling move never
// touches.
func (c CPUCgroupEnforcer) copyMemoryLimits(logger lager.Logger, fromCgroup, toCgroup string) error {
	if !cgroups.IsCgroup2UnifiedMode() {
		return nil
	}

	for _, memoryFile := range memoryLimitFiles {
		fromPath := filepath.Join(fromCgroup, memoryFile)
		limit, err := os.ReadFile(fromPath)
		if err != nil {
			// memory.swap.max is absent when swap accounting is disabled; a
			// missing source file just means there is no limit to propagate.
			if os.IsNotExist(err) {
				logger.Info("memory-limit-source-absent-skip", lager.Data{"file": fromPath})
				continue
			}
			return err
		}

		// The bad cgroup's memory.max exists because enableSupportedControllers
		// enables the memory controller on the good and bad cgroups at startup.
		// Guard the write symmetrically with the read so that, if that setup
		// ever changes, a missing target file degrades gracefully instead of
		// failing Punish and disabling CPU throttling entirely.
		toPath := filepath.Join(toCgroup, memoryFile)
		if err := os.WriteFile(toPath, limit, 0644); err != nil {
			if os.IsNotExist(err) {
				logger.Info("memory-limit-target-absent-skip", lager.Data{"file": toPath})
				continue
			}
			return err
		}
	}

	return nil
}

// clearMemoryLimits resets the bad cgroup's own memory limits back to "max"
// when a container is released from throttling. This is intentional cleanup of
// the bad cgroup's stale value, not a restore of the container's limit: once
// the PIDs move back to the good cgroup, enforcement lives there and the now
// empty bad cgroup's value is irrelevant. Resetting stops a reused bad cgroup
// from carrying a stale limit into the next punish cycle.
func (c CPUCgroupEnforcer) clearMemoryLimits(logger lager.Logger, cgroupPath string) error {
	if !cgroups.IsCgroup2UnifiedMode() {
		return nil
	}

	for _, memoryFile := range memoryLimitFiles {
		targetPath := filepath.Join(cgroupPath, memoryFile)
		if _, err := os.Stat(targetPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}

		if err := os.WriteFile(targetPath, []byte(unlimitedMemory), 0644); err != nil {
			return err
		}
	}

	return nil
}

// Runc pulls container cgroup path from the container state file
// In cgroup v1, runc is using cgroup path for device to determine container pid files
// In cgroup v2, runc is using unified cgroup path which needs to be updated
func (c CPUCgroupEnforcer) updateContainerStateCgroupPath(handle string, cgroupPath string) (retErr error) {
	if !cgroups.IsCgroup2UnifiedMode() {
		return nil
	}

	stateDir := filepath.Join(c.runcRoot, c.namespace)
	statePath := filepath.Join(stateDir, handle, "state.json")
	stateFile, err := os.Open(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer stateFile.Close()

	var state libcontainer.State
	err = json.NewDecoder(stateFile).Decode(&state)
	if err != nil {
		return err
	}

	state.CgroupPaths[""] = cgroupPath

	tmpFile, err := os.CreateTemp(stateDir, "state-")
	if err != nil {
		return err
	}

	defer func() {
		if retErr != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
		}
	}()

	err = utils.WriteJSON(tmpFile, state)
	if err != nil {
		return err
	}
	err = tmpFile.Close()
	if err != nil {
		return err
	}

	return os.Rename(tmpFile.Name(), statePath)
}

func exists(logger lager.Logger, cgroupPath string) bool {
	_, err := os.Stat(cgroupPath)
	if err == nil {
		return true
	}

	if !os.IsNotExist(err) {
		logger.Error("failed-to-stat-cgroup-path", err, lager.Data{"cgroupPath": cgroupPath})
	}

	return false
}

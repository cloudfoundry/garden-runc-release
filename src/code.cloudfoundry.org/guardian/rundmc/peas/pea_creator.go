package peas

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/gardener"
	spec "code.cloudfoundry.org/guardian/gardener/container-spec"
	"code.cloudfoundry.org/guardian/rundmc/depot"
	"code.cloudfoundry.org/guardian/rundmc/goci"
	"code.cloudfoundry.org/guardian/rundmc/runrunc"
	"code.cloudfoundry.org/guardian/rundmc/users"
	"code.cloudfoundry.org/lager/v3"
	multierror "github.com/hashicorp/go-multierror"
	uuid "github.com/nu7hatch/gouuid"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	errorwrapper "github.com/pkg/errors"
)

var RootfsPath = filepath.Join(os.TempDir(), "pea-empty-rootfs")

//counterfeiter:generate . Volumizer
type Volumizer interface {
	Create(log lager.Logger, spec garden.ContainerSpec) (specs.Spec, error)
	Destroy(log lager.Logger, handle string) error
}

//counterfeiter:generate . PidGetter
type PidGetter interface {
	GetPid(log lager.Logger, handle string) (int, error)
}

//counterfeiter:generate . PrivilegedGetter
type PrivilegedGetter interface {
	Privileged(handle string) (bool, error)
}

//counterfeiter:generate . NetworkDepot
type NetworkDepot interface {
	SetupBindMounts(log lager.Logger, handle string, privileged bool, rootfsPath string) ([]garden.BindMount, error)
	Destroy(log lager.Logger, handle string) error
}

//counterfeiter:generate . BundleGenerator
type BundleGenerator interface {
	Generate(desiredContainerSpec spec.DesiredContainerSpec) (goci.Bndl, error)
}

type ExecRunner interface {
	RunPea(
		log lager.Logger, processID string, bundle goci.Bndl, sandboxHandle string,
		pio garden.ProcessIO, tty bool, procJSON io.Reader, extraCleanup func() error,
	) (garden.Process, error)
}

type PeaCreator struct {
	Volumizer        Volumizer
	PidGetter        PidGetter
	PrivilegedGetter PrivilegedGetter
	NetworkDepot     NetworkDepot
	BundleGenerator  BundleGenerator
	BundleSaver      depot.BundleSaver
	ProcessBuilder   runrunc.ProcessBuilder
	ExecRunner       ExecRunner
	PeaCleaner       gardener.PeaCleaner
	// GroupedRunsc creates image-backed runsc subcontainers: they use virtual
	// parent namespaces rather than the Sentry's host PID, share the sandbox's
	// limits and resolve named users from the new image. Stock peas retain
	// their setns behavior and pre-resolved uid:gid users.
	GroupedRunsc bool
	// RootfsUserLookupper overrides the grouped image user lookup; nil uses
	// users.LookupImageUser.
	RootfsUserLookupper users.UserLookupper
}

func (p *PeaCreator) CreatePea(log lager.Logger, processSpec garden.ProcessSpec, procIO garden.ProcessIO, sandboxHandle string) (_ garden.Process, theErr error) {
	errs := func(action string, err error) (garden.Process, error) {
		wrappedErr := errorwrapper.Wrap(err, action)
		log.Error(action, wrappedErr)
		return nil, wrappedErr
	}

	log = log.Session("create-pea", lager.Data{})

	processID, err := generateProcessID(processSpec.ID)
	if err != nil {
		return nil, err
	}

	log.Info("creating", lager.Data{"process_id": processID})
	defer log.Info("done")

	privileged, err := p.PrivilegedGetter.Privileged(sandboxHandle)
	if err != nil {
		return errs("determining-privileged", err)
	}

	if p.GroupedRunsc && processSpec.OverrideContainerLimits != nil && *processSpec.OverrideContainerLimits != (garden.ProcessLimits{}) {
		return errs("validating-limits", errors.New("grouped runsc peas share their sandbox's CPU and memory; independent pea limits are unsupported"))
	}

	if processSpec.Dir == "" {
		processSpec.Dir = "/"
	}
	var resolvedUser *users.ExecUser
	if !p.GroupedRunsc {
		userUID, userGID, err := parseUser(processSpec.User)
		if err != nil {
			return errs("parse-user", err)
		}
		resolvedUser = &users.ExecUser{Uid: userUID, Gid: userGID}
	}

	linuxNamespaces, err := p.linuxNamespaces(log, sandboxHandle, privileged)
	if err != nil {
		return errs("determining-namespaces", err)
	}

	runtimeSpec, err := p.Volumizer.Create(log, garden.ContainerSpec{
		Handle:     processID,
		Image:      processSpec.Image,
		Privileged: privileged,
	})
	if err != nil {
		return errs("creating-volume", err)
	}
	retainVolume := false
	defer func() {
		if theErr != nil && !retainVolume {
			theErr = multierror.Append(theErr, p.Volumizer.Destroy(log, processID)).ErrorOrNil()
		}
	}()
	if runtimeSpec.Root == nil || runtimeSpec.Root.Path == "" {
		return errs("creating-volume", errors.New("image rootfs path is missing"))
	}
	if p.GroupedRunsc {
		lookupper := p.RootfsUserLookupper
		if lookupper == nil {
			lookupper = users.LookupFunc(users.LookupImageUser)
		}
		resolvedUser, err = lookupper.Lookup(runtimeSpec.Root.Path, processSpec.User)
		if err != nil {
			return errs("lookup-image-user", err)
		}
	}

	defaultBindMounts, err := p.NetworkDepot.SetupBindMounts(log, sandboxHandle, privileged, runtimeSpec.Root.Path)
	if err != nil {
		return errs("creating-bind-mount-sources", err)
	}

	if runtimeSpec.Windows == nil {
		runtimeSpec.Windows = new(specs.Windows)
	}

	runtimeSpec.Windows.Network = &specs.WindowsNetwork{
		NetworkSharedContainerName: sandboxHandle,
	}

	cgroupPath := filepath.Join(sandboxHandle, processID)

	limits := garden.Limits{}
	if processSpec.OverrideContainerLimits != nil {
		cgroupPath = processID
		limits = garden.Limits{
			CPU:    processSpec.OverrideContainerLimits.CPU,
			Memory: processSpec.OverrideContainerLimits.Memory,
		}
	}

	bndl, genErr := p.BundleGenerator.Generate(spec.DesiredContainerSpec{
		Handle:     processID,
		BaseConfig: runtimeSpec,
		CgroupPath: cgroupPath,
		Limits:     limits,
		Namespaces: linuxNamespaces,
		BindMounts: append(processSpec.BindMounts, defaultBindMounts...),
		Privileged: privileged,
	})
	if genErr != nil {
		return errs("generating-bundle", genErr)
	}
	if p.GroupedRunsc {
		// An absent OCI PID namespace joins runsc's root virtual PID namespace.
		// Empty Path instead requests a new virtual child namespace. Remove the
		// base template's PID entry too, rather than only omitting an override.
		namespaces := make([]specs.LinuxNamespace, 0, len(bndl.Namespaces()))
		for _, ns := range bndl.Namespaces() {
			if ns.Type != specs.PIDNamespace {
				namespaces = append(namespaces, ns)
			}
		}
		bndl = bndl.WithNamespaces(namespaces...)
	}

	preparedProcess := p.ProcessBuilder.BuildProcess(bndl, processSpec, resolvedUser)
	bndl = bndl.WithProcess(*preparedProcess)

	extraCleanup := func() error {
		return p.PeaCleaner.Clean(log, processID)
	}

	proc, runErr := p.ExecRunner.RunPea(log, processID, bndl, sandboxHandle, procIO, preparedProcess.Terminal, nil, extraCleanup)
	if runErr != nil {
		var runtimeResidue interface{ PeaRootfsInUse() bool }
		retainVolume = errors.As(runErr, &runtimeResidue) && runtimeResidue.PeaRootfsInUse()
		return nil, runErr
	}

	return proc, nil
}

func (p *PeaCreator) linuxNamespaces(log lager.Logger, sandboxHandle string, privileged bool) (map[string]string, error) {
	originalCtrInitPid, err := p.PidGetter.GetPid(log, sandboxHandle)
	if err != nil {
		return nil, errorwrapper.Wrap(err, "reading-ctr-pid")
	}

	linuxNamespaces := map[string]string{}
	linuxNamespaces["mount"] = ""
	linuxNamespaces["network"] = fmt.Sprintf("/proc/%d/ns/net", originalCtrInitPid)
	if p.GroupedRunsc {
		linuxNamespaces["ipc"] = ""
		linuxNamespaces["uts"] = ""
		if !privileged {
			// The gofer needs the template's image UID/GID mappings in a new
			// namespace, not the shared Sentry's one-ID host namespace.
			linuxNamespaces["user"] = ""
		}
		return linuxNamespaces, nil
	}
	linuxNamespaces["ipc"] = fmt.Sprintf("/proc/%d/ns/ipc", originalCtrInitPid)
	linuxNamespaces["pid"] = fmt.Sprintf("/proc/%d/ns/pid", originalCtrInitPid)
	linuxNamespaces["uts"] = fmt.Sprintf("/proc/%d/ns/uts", originalCtrInitPid)

	if !privileged {
		linuxNamespaces["user"] = fmt.Sprintf("/proc/%d/ns/user", originalCtrInitPid)
	}

	return linuxNamespaces, nil
}

func parseUser(uidgid string) (int, int, error) {
	if uidgid == "" {
		return 0, 0, nil
	}

	errs := func() (int, int, error) {
		return 0, 0, fmt.Errorf("'%s' is not a valid uid:gid", uidgid)
	}

	uidGidComponents := strings.Split(uidgid, ":")
	if len(uidGidComponents) != 2 {
		return errs()
	}
	uid, err := strconv.Atoi(uidGidComponents[0])
	if err != nil {
		return errs()
	}
	gid, err := strconv.Atoi(uidGidComponents[1])
	if err != nil {
		return errs()
	}
	return uid, gid, nil
}

func generateProcessID(existingID string) (string, error) {
	if existingID != "" {
		return existingID, nil
	}
	id, err := uuid.NewV4()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

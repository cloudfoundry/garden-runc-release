//go:build tools
// +build tools

package tools

import (
	_ "code.cloudfoundry.org/guardian/cmd/dadoo"
	_ "code.cloudfoundry.org/guardian/cmd/execas"
	_ "code.cloudfoundry.org/guardian/cmd/gdn"
	_ "code.cloudfoundry.org/guardian/cmd/socket2me"
	_ "code.cloudfoundry.org/guardian/cmd/winit"
	_ "code.cloudfoundry.org/guardian/grootfs"
	_ "code.cloudfoundry.org/guardian/grootfs/store/filesystems/overlayxfs/tardis"
	_ "code.cloudfoundry.org/guardian/idmapper"
	_ "code.cloudfoundry.org/guardian/idmapper/cmd/maximus"
	_ "code.cloudfoundry.org/guardian/idmapper/cmd/newgidmap"
	_ "code.cloudfoundry.org/guardian/idmapper/cmd/newuidmap"
	_ "code.cloudfoundry.org/dontpanic"
	_ "code.cloudfoundry.org/greenskeeper/cmd/greenskeeper"
	_ "github.com/onsi/ginkgo/v2/ginkgo"
	_ "github.com/opencontainers/runc"
)

// This file imports packages that are used when running go generate, or used
// during the development process but not otherwise depended on by built code.

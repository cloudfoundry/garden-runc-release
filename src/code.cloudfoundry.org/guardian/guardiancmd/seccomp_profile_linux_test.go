package guardiancmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "seccomp.json")
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatalf("writing temp profile: %v", err)
	}
	return p
}

func fallbackProfile() *specs.LinuxSeccomp {
	return &specs.LinuxSeccomp{
		DefaultAction: specs.ActErrno,
		Syscalls:      []specs.LinuxSyscall{AllowSyscall("read")},
	}
}

func TestLoadSeccompProfile_EmptyPathReturnsFallback(t *testing.T) {
	fb := fallbackProfile()
	out, err := loadSeccompProfile("", fb)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != fb {
		t.Fatal("empty path must return the compiled-in fallback unchanged")
	}
}

func TestLoadSeccompProfile_MissingFileIsError(t *testing.T) {
	if _, err := loadSeccompProfile("/no/such/seccomp.json", fallbackProfile()); err == nil {
		t.Fatal("missing file must be a hard error, not a silent fallback")
	}
}

func TestLoadSeccompProfile_InvalidJSONIsError(t *testing.T) {
	p := writeTemp(t, "{not valid json")
	if _, err := loadSeccompProfile(p, fallbackProfile()); err == nil {
		t.Fatal("invalid JSON must be a hard error")
	}
}

func TestLoadSeccompProfile_RejectsDefaultAllow(t *testing.T) {
	p := writeTemp(t, `{"defaultAction":"SCMP_ACT_ALLOW"}`)
	if _, err := loadSeccompProfile(p, fallbackProfile()); err == nil {
		t.Fatal("defaultAction SCMP_ACT_ALLOW (default-permit) must be rejected")
	}
}

func TestLoadSeccompProfile_RejectsMissingDefaultAction(t *testing.T) {
	p := writeTemp(t, `{"syscalls":[{"names":["read"],"action":"SCMP_ACT_ALLOW"}]}`)
	if _, err := loadSeccompProfile(p, fallbackProfile()); err == nil {
		t.Fatal("a profile with no defaultAction must be rejected")
	}
}

func TestLoadSeccompProfile_ReplacesWithValidProfile(t *testing.T) {
	p := writeTemp(t, `{
		"defaultAction":"SCMP_ACT_ERRNO",
		"architectures":["SCMP_ARCH_X86_64"],
		"syscalls":[{"names":["read","write"],"action":"SCMP_ACT_ALLOW"}]
	}`)
	out, err := loadSeccompProfile(p, fallbackProfile())
	if err != nil {
		t.Fatalf("valid profile must load: %v", err)
	}
	if out.DefaultAction != specs.ActErrno {
		t.Fatalf("expected DefaultAction ActErrno, got %v", out.DefaultAction)
	}
	if len(out.Syscalls) != 1 || len(out.Syscalls[0].Names) != 2 {
		t.Fatalf("loaded profile should fully replace the fallback, got %#v", out.Syscalls)
	}
	// It must be the loaded profile, not the fallback.
	for _, r := range out.Syscalls {
		for _, n := range r.Names {
			if n == "read" {
				goto found
			}
		}
	}
	t.Fatal("expected read in the replacing profile")
found:
}

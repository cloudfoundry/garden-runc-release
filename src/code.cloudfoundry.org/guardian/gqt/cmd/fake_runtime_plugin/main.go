package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/urfave/cli/v2"
)

func main() {
	fakeRuntimePlugin := cli.NewApp()
	fakeRuntimePlugin.Name = "fakeRuntimePlugin"
	fakeRuntimePlugin.Usage = "I am FakeRuntimePlugin!"

	fakeRuntimePlugin.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name: "debug",
		},
		&cli.StringFlag{
			Name: "log",
		},
		&cli.StringFlag{
			Name: "log-handle",
		},
		&cli.StringFlag{
			Name: "log-format",
		},
		&cli.StringFlag{
			Name: "image-store",
		},
		&cli.StringFlag{
			Name: "root",
		},
	}

	fakeRuntimePlugin.Commands = []*cli.Command{
		&CreateCommand,
		&RunCommand,
		&DeleteCommand,
		&StateCommand,
		&EventsCommand,
		&ExecCommand,
		&ChildProcessCommand,
	}

	_ = fakeRuntimePlugin.Run(os.Args)
}

// tempDir resolves the effective temp directory the same way the gqt test
// harness does. os.TempDir() cannot be relied on here: under some Windows
// execution contexts (e.g. LocalSystem/service accounts) GetTempPath ignores
// the process's TMP/TEMP/TMPDIR environment variables and always resolves to
// the machine-wide system temp directory, which does not match the per-test
// directory the harness expects evidence files to be written to.
func tempDir() string {
	for _, key := range []string{"TMPDIR", "TEMP", "TMP"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return os.TempDir()
}

func writeArgs(action string) {
	err := os.WriteFile(filepath.Join(tempDir(), fmt.Sprintf("%s-args", action)), []byte(strings.Join(os.Args, " ")), 0777)
	if err != nil {
		panic(err)
	}
}

func readOutput(action string) (string, bool) {
	content, err := os.ReadFile(filepath.Join(tempDir(), fmt.Sprintf("runtime-%s-output", action)))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false
		}
		panic(err)
	}
	return string(content), true
}

var CreateCommand = cli.Command{
	Name: "create",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "no-new-keyring",
		},
		&cli.StringFlag{
			Name: "bundle",
		},
		&cli.StringFlag{
			Name: "pid-file",
		},
	},

	Action: func(ctx *cli.Context) error {
		writeArgs("create")

		if err := os.WriteFile(ctx.String("pid-file"), []byte(strconv.Itoa(os.Getppid())), 0777); err != nil {
			panic(err)
		}

		return nil
	},
}

var RunCommand = cli.Command{
	Name: "run",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "no-new-keyring",
		},
		&cli.StringFlag{
			Name: "bundle, b",
		},
		&cli.StringFlag{
			Name: "pid-file",
		},
		&cli.BoolFlag{
			Name: "detach, d",
		},
	},

	Action: func(ctx *cli.Context) error {
		writeArgs("run")

		if err := os.WriteFile(ctx.String("pid-file"), []byte(strconv.Itoa(os.Getppid())), 0777); err != nil {
			panic(err)
		}

		return nil
	},
}

var DeleteCommand = cli.Command{
	Name: "delete",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "force, f",
		},
	},

	Action: func(ctx *cli.Context) error {
		writeArgs("delete")

		return nil
	},
}

var StateCommand = cli.Command{
	Name:  "state",
	Flags: []cli.Flag{},

	Action: func(ctx *cli.Context) error {
		state := `{"pid":1234, "status":"created"}`
		if overrideState, ok := readOutput("state"); ok {
			state = overrideState
		}
		fmt.Println(state)
		return nil
	},
}

var EventsCommand = cli.Command{
	Name: "events",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name: "stats",
		},
	},

	Action: func(ctx *cli.Context) error {
		fmt.Printf("{}")
		return nil
	},
}

func copyFile(source, target string) {
	sourceFile, err := os.Open(source)
	if err != nil {
		panic(err)
	}
	defer sourceFile.Close()

	targetFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		panic(err)
	}
	defer targetFile.Close()

	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		panic(err)
	}
}

var ExecCommand = cli.Command{
	Name: "exec",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "process",
			Aliases: []string{"p"},
			Usage:   "path to the process.json",
		},
		&cli.BoolFlag{
			Name:    "detach",
			Aliases: []string{"d"},
			Usage:   "detach from the container's process",
		},
		&cli.StringFlag{
			Name:  "pid-file",
			Value: "",
			Usage: "specify the file to write the process id to",
		},
	},

	Action: func(ctx *cli.Context) error {
		procSpecFilePath := filepath.Join(tempDir(), "exec-process-spec")
		copyFile(ctx.String("p"), procSpecFilePath)
		writeArgs("exec")

		var procSpec specs.Process
		procSpecFile, err := os.Open(procSpecFilePath)
		mustNot(err)
		defer procSpecFile.Close()
		must(json.NewDecoder(procSpecFile).Decode(&procSpec))

		exitCodeStr := procSpec.Args[1]
		exitCode, err := strconv.Atoi(exitCodeStr)
		mustNot(err)

		stdoutStr := procSpec.Args[2]
		_, err = fmt.Fprintln(os.Stdout, stdoutStr)
		mustNot(err)

		stderrStr := procSpec.Args[3]
		_, err = fmt.Fprintln(os.Stderr, stderrStr)
		mustNot(err)

		// To satisfy dadoo's requirement that the runtime plugin fork SOMETHING
		childCmd := exec.Command(os.Args[0], "child", "--exitcode", exitCodeStr)
		must(childCmd.Start())
		childPid := childCmd.Process.Pid
		must(os.WriteFile(ctx.String("pid-file"), []byte(fmt.Sprintf("%d", childPid)), 0777))

		os.Exit(exitCode)

		return nil
	},
}

// Forked as external process by exec subcmd
var ChildProcessCommand = cli.Command{
	Name: "child",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name: "exitcode",
		},
	},
	Action: func(ctx *cli.Context) error {
		os.Exit(ctx.Int("exitcode"))
		return nil
	},
}

func mustNot(err error) {
	if err != nil {
		panic(err)
	}
}

var must = mustNot

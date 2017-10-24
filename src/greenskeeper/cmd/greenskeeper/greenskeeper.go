package main

import (
	"fmt"
	"greenskeeper"
	"os"
)

func main() {
	pidFilePath := os.Getenv("PIDFILE")
	if err := greenskeeper.CheckExistingGdnProcess(pidFilePath); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	directories := []greenskeeper.Directory{
		greenskeeper.NewDirectoryBuilder(mustGetenv("RUN_DIR")).Build(),
		greenskeeper.NewDirectoryBuilder(mustGetenv("GARDEN_DIR")).Mode(0770).Build(),
		greenskeeper.NewDirectoryBuilder(mustGetenv("LOG_DIR")).Build(),
		greenskeeper.NewDirectoryBuilder(mustGetenv("TMPDIR")).Build(),
		greenskeeper.NewDirectoryBuilder(mustGetenv("DEPOT_PATH")).Build(),
		greenskeeper.NewDirectoryBuilder(mustGetenv("RUNTIME_BIN_DIR")).Mode(0750).Build(),
		greenskeeper.NewDirectoryBuilder(mustGetenv("GRAPH_PATH")).Mode(0700).Build(),
	}

	if err := greenskeeper.CreateDirectories(directories...); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func mustGetenv(key string) string {
	env := os.Getenv(key)
	if env == "" {
		fmt.Fprintf(os.Stderr, "expected environment variable %s to be set", key)
		os.Exit(1)
	}

	return env
}

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
	os.Exit(0)
}

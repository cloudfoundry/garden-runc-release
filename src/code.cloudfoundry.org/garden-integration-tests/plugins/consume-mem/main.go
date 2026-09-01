package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	mem, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Println("bad arg: " + err.Error())
	}

	a := make([]byte, mem)
	fmt.Printf("Allocated %d\n", len(a))
	os.Exit(0)
}

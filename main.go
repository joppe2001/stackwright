package main

import (
	"fmt"
	"os"

	"github.com/joppe2001/stackwright/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "stackwright:", err)
		os.Exit(1)
	}
}

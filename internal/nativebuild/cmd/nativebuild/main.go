package main

import (
	"fmt"
	"os"

	"sshc/internal/nativebuild"
)

func main() {
	if err := nativebuild.RunNativeBuild(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "native build:", err)
		os.Exit(1)
	}
}

package main

import (
	"fmt"
	"os"

	"sshc/internal/buildcontract"
)

func main() {
	if err := buildcontract.RunNativeBuild(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "native build:", err)
		os.Exit(1)
	}
}

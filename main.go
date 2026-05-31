package main

import (
	"fmt"
	"runtime/debug"
)

// Version is the application version, injected at build time via ldflags
var Version = "dev"

func main() {
	if Version == "dev" || Version == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
				Version = bi.Main.Version
			}
		}
	}

	fmt.Printf("invdex %s\n", Version)
	fmt.Println("A personal inventory system powered by LLMs.")
}

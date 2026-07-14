package main

import (
	"fmt"
	"os"

	"github.com/willtanoe/raid/internal/raid"
)

func main() {
	if err := raid.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "raid:", err)
		os.Exit(1)
	}
}

// Command runner_fixture is a small test binary used by runner_test.go.
//
// Modes (chosen by argv[1]):
//
//	exit-zero     prints "ready" and exits 0
//	exit-1        prints "fail"  and exits 1
//	exit-75       prints "rotate" and exits 75 (plugin rotation contract)
//	ignore-sigterm prints "running", ignores SIGTERM forever (drain timeout test)
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	mode := "exit-zero"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	switch mode {
	case "exit-zero":
		fmt.Println("ready")
		os.Exit(0)
	case "exit-1":
		fmt.Println("fail")
		os.Exit(1)
	case "exit-75":
		fmt.Println("rotate")
		os.Exit(75)
	case "ignore-sigterm":
		fmt.Println("running")
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM)
		// Block forever even on SIGTERM. The runner's drain timeout
		// should escalate to SIGKILL.
		for {
			<-ch
			time.Sleep(time.Hour)
		}
	default:
		fmt.Fprintf(os.Stderr, "runner_fixture: unknown mode %q\n", mode)
		os.Exit(2)
	}
}

// simulate-lifecycle runs only fictional local fixtures. It is not wired into
// the API, scheduler, production image, provider clients, or risk approval.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/arbion/platform/services/api/internal/executionsim"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "This offline fixture command accepts no accounts, credentials, or trade instructions.")
		os.Exit(2)
	}
	directory, err := os.MkdirTemp("", "arbion-simulation-lifecycle-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Unable to create private simulation journal directory.")
		os.Exit(1)
	}
	fmt.Println("OFFLINE FIXTURE LAB — fictional prices and balances; no broker or AI calls.")
	for _, provider := range []string{"coinbase", "schwab"} {
		s, err := executionsim.RunScenario(filepath.Join(directory, provider+".jsonl"), provider, time.Now().UTC())
		if err != nil {
			fmt.Fprintln(os.Stderr, "Simulation failed closed:", err)
			os.Exit(1)
		}
		fmt.Println(executionsim.Describe(s))
	}
	fmt.Println("Verified restart replay, no resend after unknown outcome, partial/cancel race, duplicate fills and transfers, exact cash/quantity, and terminal rejection.")
	fmt.Println("Synthetic journals retained at:", directory)
}

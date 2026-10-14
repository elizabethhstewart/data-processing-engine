package main

import (
	"baltimore-insights/internal/analysis"
	"flag"
	"fmt"
	"os"
	"runtime"
)

func main() {
	crime := flag.String("crime", "data/crime.csv", "crime CSV")
	vacant := flag.String("vacant", "data/vacant.csv", "vacant notice CSV")
	year := flag.Int("year", 2025, "crime analysis year")
	workers := flag.Int("workers", min(runtime.NumCPU(), 8), "workers per dataset (1–64)")
	top := flag.Int("top", 10, "neighborhoods per ranking")
	flag.Parse()
	if *top < 1 {
		fmt.Fprintln(os.Stderr, "top must be positive")
		os.Exit(1)
	}
	r, err := analysis.Load(*crime, *vacant, *year, *workers)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	analysis.Report(os.Stdout, r, *year, *top)
}

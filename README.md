# Baltimore Crime and Vacant Building Insights

A Go command-line application for a fictional Baltimore City Planning team. It joins public crime and vacant building notice records by normalized neighborhood and produces five research summaries with tables and a monthly ASCII chart.

## Build and run on macOS

Install Go 1.22 or later from https://go.dev/dl/ (choose the macOS installer matching Apple Silicon or Intel), then open a new terminal:

```sh
go version
cd data-processing-engine
go test -race ./...
go build -o bin/baltimore ./cmd/baltimore
mkdir -p data reports
```

Download the raw CSV exports from the sources below. Save crime data as `data/crime.csv` and vacant notices as `data/vacant.csv`. The supplied CSV inputs are included in this repository so a clone can run immediately. Generated reports and binaries are ignored by Git. The provided crime export has 54,076 rows, predominantly in 2025; the vacancy export has 11,445 rows. No API credentials or external Go libraries are required.

```sh
./bin/baltimore -year 2025 -workers 4 -top 10 > reports/2025.txt
cat reports/2025.txt
```

Use `-crime /path/to/crime.csv` and `-vacant /path/to/vacant.csv` for other locations. `-workers` sets workers per dataset (1–64); the default is the smaller of eight and the logical CPU count. The program exits unsuccessfully on missing columns, malformed CSV or invalid dates. It prints no partial report when ingestion fails.

## Run on Windows with VS Code

Install Go from https://go.dev/dl/ using the Windows installer, then restart VS Code. Open the repository folder and use Terminal → New Terminal:

```powershell
go version
go test ./...
go run ./cmd/baltimore -year 2025 -workers 4 -top 10
```

The CSV files are already in `data`. The ordinary tests require no C compiler; Windows race-detector tests additionally require a compatible C compiler and CGO configuration. macOS build instructions above remain part of the assignment submission.

## Sources and required columns

Source links supplied with the datasets:

- https://data.baltimorecity.gov/datasets/204beefe92a645d79fdf0969957bbdf8_0/explore
- https://data.baltimorecity.gov/datasets/691d65a5f85640e6aaa46930bd9dc102_1/explore

Portal metadata could not yet be accessed in this environment; attribution of each item ID to its export and the vacancy coordinate system still need verification. Inputs inspected: `NIBRS_GroupA_Crime_Data_-9093015555270796969.csv` and `Vacant_Building_Notices.csv`.

Crime requires `Neighborhood`, `CrimeDateTime`, `CCNumber`, `Description`. Crime dates use `1/2/2006 3:04:05 PM`. Vacancies require `Neighborhood`, `DateNotice`, `BLOCKLOT`; dates use `2006/01/02 15:04:05-07`. Matching trims whitespace, collapses internal whitespace and uppercases names. It does not infer missing neighborhoods or perform fuzzy/geospatial matching.

## Five research questions

1. Which neighborhoods have the most vacancy notices, and how many crime records and distinct cases are reported there?
2. Which neighborhoods with notices have the most distinct crime cases?
3. What crime types are reported in neighborhoods with versus without notices?
4. How do monthly crime record counts vary in neighborhoods with notices?
5. Which neighborhoods rank highest on a transparent combined planning score?

The score is `50 × notices / maximum notices + 50 × neighborhood cases / maximum neighborhood cases`, using maxima across all input neighborhoods. Rankings break ties alphabetically. This score is exploratory, not a prediction or a recommendation to allocate enforcement resources.

## Architecture, concurrency and memory

The Pipeline pattern separates CSV ingestion, row parsing/normalization, neighborhood matching and aggregation, and output formatting. `cmd/baltimore` handles flags; `internal/analysis` contains the pipeline and reports.

Two readers ingest datasets concurrently. Each feeds a bounded channel of 256 rows into a configurable parsing worker pool. Parsed events enter another bounded channel of 256 events. A single collector owns aggregation maps, avoiding shared map writes; worker errors are protected by a mutex. Wait groups close channels after producers finish. Backpressure limits queued rows; the full CSV is never loaded into memory.

Go's garbage collector manages row allocations. Aggregate memory grows with distinct neighborhoods, crime types and distinct case IDs, rather than all raw rows. Case sets remain on the heap for exact counting. Worker counts are bounded to avoid unbounded goroutine overhead; more workers are not guaranteed to be faster. The default caps each pool at eight.

## Interpretation and data quality

- Crime rows are records, not necessarily unique incidents: the supplied file has 47,289 distinct case numbers across all years. Each neighborhood counts distinct `CCNumber` values independently; a case can occur in multiple neighborhoods. Do not sum neighborhood case counts as a citywide unique total.
- Crime is filtered by the parsed calendar year, excluding the January 1, 2026 boundary records from a 2025 analysis.
- Notices are a supplied snapshot, including notices dated after 2025. The application reports that count and does not reconstruct historical vacancy status. All cancellation and abatement dates in the supplied export are blank.
- Notice counts count rows. The supplied export has one row per distinct parcel; replacement inputs must be checked for duplicate notices before treating counts as buildings.
- All 213 vacancy neighborhoods match a crime neighborhood after normalization in these exports. Missing crime neighborhoods are reported and excluded from neighborhood summaries. Missing vacancy neighborhoods are excluded from neighborhood summaries but included in the total.
- Counts are not population-adjusted crime rates. The with/without comparison is not adjusted for population, neighborhood area, or group size. Associations cannot establish causation.
- No distance-based results are produced. The vacancy X/Y values differ from longitude/latitude and require authoritative coordinate-system verification.

## Verification

`go test -race ./...` checks concurrent parsing with one and four workers, normalization, repeated case IDs, year boundaries, missing neighborhoods, report generation and invalid inputs. The race detector checks executed paths for unsafe memory access. Run the real-data command above to verify ingestion of your exports. Optional: compare output using `-workers 1` versus `-workers 4`; results should be identical.

// Command prepare-airports regenerates the embedded airports dataset from the
// raw OurAirports dump. It lives in its own module so its only heavy
// dependency — tzf, the lat/lon → IANA timezone resolver — never enters the
// main module: timezones are resolved here, once, at dataset-preparation time,
// and the service binary just reads the tz column.
//
// Usage (from the repo root; output path is the embedded dataset):
//
//	curl -sSLo /tmp/airports-raw.csv https://davidmegginson.github.io/ourairports-data/airports.csv
//	cd scripts/prepare-airports && go run . /tmp/airports-raw.csv ../../internal/services/airports/data/airports.csv
//
// Kept rows: type large/medium/small_airport with a well-formed IATA code —
// heliports, seaplane bases, and closed fields are noise for a flight-price
// watcher. Kept columns: iata, type, name, municipality, iso_country,
// latitude, longitude, scheduled, tz.
package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"

	"github.com/ringsaturn/tzf"
)

var iataRe = regexp.MustCompile(`^[A-Z]{3}$`)

func main() {
	if len(os.Args) != 3 {
		log.Fatalf("usage: %s <ourairports-airports.csv> <output.csv>", os.Args[0])
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		log.Fatal(err)
	}
}

func run(inPath, outPath string) error {
	in, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer in.Close()

	r := csv.NewReader(in)
	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	col := map[string]int{}
	for i, name := range header {
		col[name] = i
	}
	for _, need := range []string{"type", "name", "latitude_deg", "longitude_deg", "iso_country", "municipality", "scheduled_service", "iata_code"} {
		if _, ok := col[need]; !ok {
			return fmt.Errorf("input is missing column %q — not an OurAirports airports.csv?", need)
		}
	}

	finder, err := tzf.NewDefaultFinder()
	if err != nil {
		return fmt.Errorf("init timezone finder: %w", err)
	}

	rows, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("read rows: %w", err)
	}

	var out [][]string
	for _, row := range rows {
		typ, iata := row[col["type"]], row[col["iata_code"]]
		if !iataRe.MatchString(iata) {
			continue
		}
		if typ != "large_airport" && typ != "medium_airport" && typ != "small_airport" {
			continue
		}
		lat, errLat := strconv.ParseFloat(row[col["latitude_deg"]], 64)
		lon, errLon := strconv.ParseFloat(row[col["longitude_deg"]], 64)
		if errLat != nil || errLon != nil {
			log.Printf("skip %s: bad coordinates", iata)
			continue
		}
		tz := finder.GetTimezoneName(lon, lat)
		if tz == "" {
			log.Printf("skip %s: no timezone for %.4f,%.4f", iata, lat, lon)
			continue
		}
		out = append(out, []string{
			iata,
			typ,
			row[col["name"]],
			row[col["municipality"]],
			row[col["iso_country"]],
			strconv.FormatFloat(lat, 'f', 4, 64),
			strconv.FormatFloat(lon, 'f', 4, 64),
			row[col["scheduled_service"]],
			tz,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	_ = w.Write([]string{"iata", "type", "name", "municipality", "iso_country", "latitude", "longitude", "scheduled", "tz"})
	if err := w.WriteAll(out); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	w.Flush()
	if err := f.Close(); err != nil {
		return err
	}
	log.Printf("wrote %d airports to %s", len(out), outPath)
	return nil
}

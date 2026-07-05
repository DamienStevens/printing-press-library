package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"grants-pp-cli/internal/sources"
)

func cmdSearch(args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	closingBefore := fs.String("closing-before", "", "csak eddig a határidőig (YYYY-MM-DD)")
	agency := fs.String("agency", "", "ügynökség-kód, pl. HHS-NIH11, NSF")
	rows := fs.Int("rows", 15, "találatok száma")
	details := fs.Bool("details", false, "keret + jogosultság lekérése")
	minAward := fs.Int64("min-award", 0, "min. keretösszeg USD (részlet-lekéréssel jár)")
	eligibility := fs.String("eligibility", "", "jogosultság-szűrő, substring")
	asJSON := fs.Bool("json", false, "JSON kimenet")
	pos, err := parseFlexible(fs, args)
	if err != nil {
		return 2
	}
	keyword := strings.Join(pos, " ")
	if keyword == "" {
		fmt.Fprintln(os.Stderr, "kell egy kulcsszó / a keyword is required: grants-pp-cli search <kulcsszó>")
		return 2
	}

	var cutoff time.Time
	if *closingBefore != "" {
		cutoff, err = time.Parse("2006-01-02", *closingBefore)
		if err != nil {
			return fail(fmt.Errorf("--closing-before formátum: YYYY-MM-DD (kaptam: %q)", *closingBefore))
		}
	}

	opps, total, err := sources.SearchOpportunities(keyword, *agency, *rows)
	if err != nil {
		return fail(err)
	}
	if !cutoff.IsZero() {
		opps = ClosingBefore(opps, cutoff)
	}

	needDetails := *details || *minAward > 0 || *eligibility != ""
	if needDetails {
		var kept []sources.Opportunity
		for _, o := range opps {
			d, derr := sources.FetchDetails(o.ID)
			if derr != nil {
				fmt.Fprintf(os.Stderr, "  (figyelem / warn: %v — sor kihagyva)\n", derr)
				continue
			}
			o.Details = d
			if *minAward > 0 && d.AwardCap() < *minAward {
				continue
			}
			if !EligibilityMatches(d.ApplicantTypes, *eligibility) {
				continue
			}
			kept = append(kept, o)
		}
		opps = kept
	}

	if *asJSON {
		return printJSON(map[string]any{"keyword": keyword, "totalHits": total, "shown": len(opps), "opportunities": opps})
	}

	fmt.Printf("🔎 %q — %d nyitott kiírás összesen, %d mutatva", keyword, total, len(opps))
	if !cutoff.IsZero() {
		fmt.Printf(" (határidő ≤ %s)", cutoff.Format("2006-01-02"))
	}
	fmt.Println()
	for _, o := range opps {
		fmt.Printf("  %-14s %-9s zárás: %-10s  %s\n", o.Number, o.AgencyCode, o.CloseDate, truncate(o.Title, 70))
		if o.Details != nil {
			label := "keret"
			if o.Details.AwardCeiling == 0 && o.Details.EstimatedFunding > 0 {
				label = "becsült" // estimatedFunding, nem hard ceiling
			}
			fmt.Printf("  %14s %s: %s", "", label, FormatMoney(o.Details.AwardCap()))
			if len(o.Details.ApplicantTypes) > 0 {
				fmt.Printf("  · jogosult: %s", truncate(strings.Join(o.Details.ApplicantTypes, "; "), 80))
			}
			fmt.Println()
		}
	}
	if len(opps) == 0 {
		fmt.Println("  (nincs találat a szűrőkkel / no results with these filters)")
	}
	return 0
}

func printJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fail(err)
	}
	return 0
}

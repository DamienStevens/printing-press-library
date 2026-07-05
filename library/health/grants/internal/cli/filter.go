package cli

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"grants-pp-cli/internal/sources"
)

// parseFlexible: flag.Parse megáll az első pozicionális argumentumnál — ezért a
// `search kulcsszó --rows 5` alakban a flagek a kulcsszóba folynának. Itt
// újra-parszolással kiszedjük a pozicionálisokat, a flagek bárhol állhatnak.
func parseFlexible(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
	return pos, nil
}

// Tiszta, tesztelhető szűrő-logika / pure, testable filter logic.

const grantsDateLayout = "01/02/2006" // Grants.gov: MM/DD/YYYY

// ParseGrantsDate parses a Grants.gov MM/DD/YYYY date.
func ParseGrantsDate(s string) (time.Time, error) {
	return time.Parse(grantsDateLayout, strings.TrimSpace(s))
}

// ClosingBefore keeps opportunities whose close date is valid and on/before cutoff.
func ClosingBefore(opps []sources.Opportunity, cutoff time.Time) []sources.Opportunity {
	var out []sources.Opportunity
	for _, o := range opps {
		d, err := ParseGrantsDate(o.CloseDate)
		if err != nil {
			continue // nincs értelmezhető határidő → kiszűrjük, mert a feltétel határidőre kérdez
		}
		if !d.After(cutoff) {
			out = append(out, o)
		}
	}
	return out
}

// EligibilityMatches: case-insensitive substring bármely applicant type leírásban.
func EligibilityMatches(types []string, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	for _, t := range types {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

// FormatMoney: 1234567 → "$1,234,567"; 0 → "—".
func FormatMoney(n int64) string {
	if n <= 0 {
		return "—"
	}
	s := fmt.Sprint(n)
	var b strings.Builder
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
	}
	for i := pre; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return "$" + b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

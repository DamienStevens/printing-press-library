package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/readings"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/source"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/store"

	"github.com/spf13/cobra"
)

// Health thresholds. These are conservative, widely-cited defaults — easy to
// reason about, not regulatory gospel. Documented here so the dashboard's
// judgments are auditable rather than buried magic numbers.
const (
	moldRHThreshold = 60.0   // % RH; sustained moisture above this grows mold
	radonActionBq   = 100.0  // Bq/m³; WHO reference action level
	pm25ElevatedUg  = 12.0   // µg/m³; EPA annual PM2.5 guideline
	co2StuffyPpm    = 1000.0 // ppm; common indoor-air "ventilation" line
	vocElevatedPpb  = 250.0  // ppb; AirThings "fair/poor" boundary
)

// periodSince maps a named window to its start time, using calendar arithmetic
// so "month"/"quarter"/"year" mean real calendar spans, not fixed day counts.
// "all" returns the zero time (unbounded).
func periodSince(period string, now time.Time) (time.Time, error) {
	switch period {
	case "day":
		return now.AddDate(0, 0, -1), nil
	case "week":
		return now.AddDate(0, 0, -7), nil
	case "month":
		return now.AddDate(0, -1, 0), nil
	case "quarter":
		return now.AddDate(0, -3, 0), nil
	case "year":
		return now.AddDate(-1, 0, 0), nil
	case "all", "":
		return time.Time{}, nil
	default:
		return time.Time{}, fmt.Errorf("unknown period %q (use day|week|month|quarter|year|all)", period)
	}
}

// dashboardJSON is the agent-facing structured view.
type dashboardJSON struct {
	Period   string               `json:"period"`
	Focus    string               `json:"focus"`
	From     *time.Time           `json:"from,omitempty"`
	To       time.Time            `json:"to"`
	Readings int                  `json:"readings"`
	Rooms    []string             `json:"rooms"`
	Metrics  []readings.AggRow    `json:"metrics"`
	Mold     []readings.ExceedRow `json:"mold,omitempty"`
	Radon    []readings.AggRow    `json:"radon,omitempty"`
	Flags    []string             `json:"flags"`
}

func newDashboardCmd(flags *rootFlags) *cobra.Command {
	var period, focus string
	var room, metric, sourceFilter []string
	var noSync bool
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Whole-home air-health view across every sensor, over any time window",
		Long: "Aggregates every sensor (AirThings, IQAir/AirVisual, MOCREO) into one view for the\n" +
			"chosen period, with a mold or allergy focus. Refreshes the latest readings first\n" +
			"unless --no-sync. Long windows reflect whatever history has accrued locally.",
		Example: "  home-health-pp-cli dashboard --period week --focus mold\n" +
			"  home-health-pp-cli dashboard --period quarter --focus allergy\n" +
			"  home-health-pp-cli dashboard --period day --room Bedroom,Crawlspace\n" +
			"  home-health-pp-cli dashboard --period month --json --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			now := time.Now()
			since, err := periodSince(period, now)
			if err != nil {
				return err
			}
			switch focus {
			case "mold", "allergy", "all", "":
			default:
				return fmt.Errorf("unknown focus %q (use mold|allergy|all)", focus)
			}

			st, err := openReadings(ctx)
			if err != nil {
				return err
			}
			defer st.Close()

			if !noSync {
				// Light refresh so the latest snapshot is current; ignore
				// per-source failures here (the view still renders stored data).
				_ = collectAll(ctx, st, now.Add(-2*time.Hour))
			}

			f := readings.Filter{Since: since, Rooms: room, Metrics: metric, Sources: sourceFilter}
			view, err := buildDashboard(ctx, st, f, period, focus, now)
			if err != nil {
				return err
			}

			if flags.asJSON || flags.agent {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			renderDashboard(cmd, view)
			return nil
		},
	}
	cmd.Flags().StringVar(&period, "period", "week", "Time window: day|week|month|quarter|year|all")
	cmd.Flags().StringVar(&focus, "focus", "all", "Health lens: mold|allergy|all")
	cmd.Flags().StringSliceVar(&room, "room", nil, "Filter to these rooms (comma-separated)")
	cmd.Flags().StringSliceVar(&metric, "metric", nil, "Filter to these metrics")
	cmd.Flags().StringSliceVar(&sourceFilter, "source", nil, "Filter to these sources (airthings|iqair|mocreo)")
	cmd.Flags().BoolVar(&noSync, "no-sync", false, "Skip the pre-render refresh; report stored data only")
	return cmd
}

func buildDashboard(ctx context.Context, st *store.Store, f readings.Filter, period, focus string, now time.Time) (dashboardJSON, error) {
	if focus == "" {
		focus = "all"
	}
	view := dashboardJSON{Period: orDefault(period, "week"), Focus: focus, To: now}
	if !f.Since.IsZero() {
		s := f.Since
		view.From = &s
	}
	rooms, err := readings.Distinct(ctx, st.DB(), "room", f)
	if err != nil {
		return view, err
	}
	view.Rooms = rooms

	agg, err := readings.Aggregate(ctx, st.DB(), f)
	if err != nil {
		return view, err
	}
	_, _, count, _ := readings.Span(ctx, st.DB(), f)
	view.Readings = count

	// Metric view depends on focus.
	switch focus {
	case "mold":
		view.Metrics = filterMetrics(agg, source.MetricHumidity, source.MetricTemp, source.MetricMold)
	case "allergy":
		view.Metrics = filterMetrics(agg, source.MetricPM25, source.MetricVOC, source.MetricCO2, source.MetricPM10)
	default:
		view.Metrics = agg
	}

	// Mold exceedance is always computed for mold/all.
	if focus == "mold" || focus == "all" {
		ex, err := readings.Exceedance(ctx, st.DB(), source.MetricHumidity, moldRHThreshold, f)
		if err != nil {
			return view, err
		}
		view.Mold = ex
	}
	// Radon hazard tile (always surfaced — it's a standalone risk).
	view.Radon = filterMetrics(agg, source.MetricRadon)

	view.Flags = computeFlags(agg, view.Mold)
	return view, nil
}

// computeFlags turns the numbers into plain-language warnings an agent or human
// can act on, applying the documented thresholds.
func computeFlags(agg []readings.AggRow, mold []readings.ExceedRow) []string {
	var flags []string
	for _, e := range mold {
		if e.Over > 0 {
			flags = append(flags, fmt.Sprintf("MOLD RISK: %s spent %.0f%% of samples ≥%.0f%% RH (peak %.1f%%)",
				e.Room, e.Fraction*100, moldRHThreshold, e.MaxValue))
		}
	}
	for _, a := range agg {
		switch a.Metric {
		case source.MetricRadon:
			if a.Max >= radonActionBq {
				flags = append(flags, fmt.Sprintf("RADON: %s reached %.0f Bq/m³ (WHO action %.0f)", a.Room, a.Max, radonActionBq))
			}
		case source.MetricPM25:
			if a.Max > pm25ElevatedUg {
				flags = append(flags, fmt.Sprintf("PARTICULATES: %s PM2.5 peaked %.0f µg/m³ (guideline %.0f)", a.Room, a.Max, pm25ElevatedUg))
			}
		case source.MetricCO2:
			if a.Max >= co2StuffyPpm {
				flags = append(flags, fmt.Sprintf("VENTILATION: %s CO₂ peaked %.0f ppm (stuffy ≥%.0f)", a.Room, a.Max, co2StuffyPpm))
			}
		case source.MetricVOC:
			if a.Max >= vocElevatedPpb {
				flags = append(flags, fmt.Sprintf("VOC: %s peaked %.0f ppb (elevated ≥%.0f)", a.Room, a.Max, vocElevatedPpb))
			}
		}
	}
	sort.Strings(flags)
	return flags
}

func filterMetrics(agg []readings.AggRow, metrics ...string) []readings.AggRow {
	keep := map[string]bool{}
	for _, m := range metrics {
		keep[m] = true
	}
	var out []readings.AggRow
	for _, a := range agg {
		if keep[a.Metric] {
			out = append(out, a)
		}
	}
	return out
}

func renderDashboard(cmd *cobra.Command, v dashboardJSON) {
	out := cmd.OutOrStdout()
	span := "all time"
	if v.From != nil {
		span = fmt.Sprintf("%s → %s", v.From.Local().Format("Jan 2 15:04"), v.To.Local().Format("Jan 2 15:04"))
	}
	fmt.Fprintf(out, "Home Health — %s (%s), focus: %s\n", strings.Title(v.Period), span, v.Focus)
	fmt.Fprintf(out, "%d readings across %d rooms\n", v.Readings, len(v.Rooms))

	if len(v.Flags) > 0 {
		fmt.Fprintln(out, "\n⚠  Attention:")
		for _, fl := range v.Flags {
			fmt.Fprintf(out, "   • %s\n", fl)
		}
	} else {
		fmt.Fprintln(out, "\n✓ No threshold exceedances in this window.")
	}

	if len(v.Mold) > 0 && (v.Focus == "mold" || v.Focus == "all") {
		fmt.Fprintln(out, "\nMold signal (humidity ≥60% RH):")
		for _, e := range v.Mold {
			if e.Total == 0 {
				continue
			}
			fmt.Fprintf(out, "   %-16s %3.0f%% of samples over, peak %.1f%%\n", e.Room, e.Fraction*100, e.MaxValue)
		}
	}

	if len(v.Metrics) > 0 {
		fmt.Fprintln(out, "\nReadings:")
		fmt.Fprintf(out, "   %-16s %-9s %8s %8s %8s\n", "Room", "Metric", "Last", "Min", "Max")
		for _, a := range v.Metrics {
			fmt.Fprintf(out, "   %-16s %-9s %7.1f%s %7.1f %7.1f\n", a.Room, a.Metric, a.Last, a.Unit, a.Min, a.Max)
		}
	}
	if len(v.Radon) > 0 {
		fmt.Fprintln(out, "\nRadon:")
		for _, a := range v.Radon {
			fmt.Fprintf(out, "   %-16s %.0f Bq/m³ (avg %.0f, peak %.0f)\n", a.Room, a.Last, a.Avg, a.Max)
		}
	}
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/readings"

	"github.com/spf13/cobra"
)

// roomSummary is one room's latest snapshot across all its metrics.
type roomSummary struct {
	Room     string             `json:"room"`
	Sources  []string           `json:"sources"`
	Latest   map[string]float64 `json:"latest"`
	Units    map[string]string  `json:"units"`
	LastSeen time.Time          `json:"last_seen"`
}

func newRoomsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "rooms",
		Short:       "List every monitored room with its latest readings",
		Example:     "  home-health-pp-cli rooms\n  home-health-pp-cli rooms --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			st, err := openReadings(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			agg, err := readings.Aggregate(ctx, st.DB(), readings.Filter{})
			if err != nil {
				return err
			}
			byRoom := map[string]*roomSummary{}
			var order []string
			for _, a := range agg {
				rs := byRoom[a.Room]
				if rs == nil {
					rs = &roomSummary{Room: a.Room, Latest: map[string]float64{}, Units: map[string]string{}}
					byRoom[a.Room] = rs
					order = append(order, a.Room)
				}
				rs.Latest[a.Metric] = a.Last
				rs.Units[a.Metric] = a.Unit
				if !contains(rs.Sources, a.Source) {
					rs.Sources = append(rs.Sources, a.Source)
				}
				if a.LastTS.After(rs.LastSeen) {
					rs.LastSeen = a.LastTS
				}
			}
			sort.Strings(order)
			summaries := make([]roomSummary, 0, len(order))
			for _, r := range order {
				summaries = append(summaries, *byRoom[r])
			}
			if flags.asJSON || flags.agent {
				return printJSONFiltered(cmd.OutOrStdout(), summaries, flags)
			}
			out := cmd.OutOrStdout()
			if len(summaries) == 0 {
				fmt.Fprintln(out, "No readings yet — run `home-health-pp-cli collect` first.")
				return nil
			}
			for _, s := range summaries {
				var parts []string
				for m, v := range s.Latest {
					parts = append(parts, fmt.Sprintf("%s=%.1f%s", m, v, s.Units[m]))
				}
				sort.Strings(parts)
				fmt.Fprintf(out, "%-16s [%s]  %s\n", s.Room, strings.Join(s.Sources, ","), strings.Join(parts, " "))
			}
			return nil
		},
	}
	return cmd
}

func newSensorCmd(flags *rootFlags) *cobra.Command {
	var period string
	cmd := &cobra.Command{
		Use:         "sensor <room>",
		Short:       "Detailed readings for one room over a time window",
		Example:     "  home-health-pp-cli sensor Crawlspace --period month\n  home-health-pp-cli sensor Bedroom --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			ctx := cmd.Context()
			since, err := periodSince(period, time.Now())
			if err != nil {
				return err
			}
			st, err := openReadings(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			room := strings.TrimSpace(args[0])
			agg, err := readings.Aggregate(ctx, st.DB(), readings.Filter{Since: since, Rooms: []string{room}})
			if err != nil {
				return err
			}
			// An unknown room (or no data yet) is a usage error, not an empty
			// success — return non-zero so scripts and agents can tell the
			// difference between "this room has no readings" and "ok, nothing".
			if len(agg) == 0 {
				return fmt.Errorf("no readings for room %q in this window — run `home-health-pp-cli collect`, or see `rooms` for known rooms", room)
			}
			if flags.asJSON || flags.agent {
				return printJSONFiltered(cmd.OutOrStdout(), agg, flags)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s — %s\n", args[0], orDefault(period, "week"))
			fmt.Fprintf(out, "   %-9s %8s %8s %8s %8s %6s\n", "Metric", "Last", "Avg", "Min", "Max", "N")
			for _, a := range agg {
				fmt.Fprintf(out, "   %-9s %7.1f%s %7.1f %7.1f %7.1f %6d\n", a.Metric, a.Last, a.Unit, a.Avg, a.Min, a.Max, a.Count)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&period, "period", "week", "Time window: day|week|month|quarter|year|all")
	return cmd
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

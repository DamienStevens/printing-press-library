package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/readings"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/source"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/source/airthings"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/source/iqair"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/source/mocreo"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/store"

	"github.com/spf13/cobra"
)

// hhDBName is the slug used for the shared SQLite database that both the
// generated store and the unified readings table live in.
const hhDBName = "home-health-pp-cli"

// allSources returns every home-health data source. AirVisual Pro local-SMB is
// covered today via the IQAir cloud source ("Home"); add an smb source here
// when a pure-LAN read is wired.
func allSources() []source.Source {
	return []source.Source{mocreo.New(), airthings.New(), iqair.New()}
}

// openReadings opens the shared DB and guarantees the readings table exists.
func openReadings(ctx context.Context) (*store.Store, error) {
	st, err := store.OpenWithContext(ctx, defaultDBPath(hhDBName))
	if err != nil {
		return nil, err
	}
	if err := readings.EnsureSchema(ctx, st.DB()); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

// collectResult is the per-source outcome of a collection pass.
type collectResult struct {
	Source    string `json:"source"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Fetched   int    `json:"fetched"`
	Inserted  int    `json:"inserted"`
	Error     string `json:"error,omitempty"`
}

// collectAll fetches from every available source and writes into the readings
// store. Each source is independent: one source failing (auth, throttle, an
// offline device) never aborts the others — its error is captured in its row so
// "no data" is always distinguishable from "this source broke".
func collectAll(ctx context.Context, st *store.Store, since time.Time) []collectResult {
	srcs := allSources()
	out := make([]collectResult, 0, len(srcs))
	for _, s := range srcs {
		res := collectResult{Source: s.Name()}
		ok, why := s.Available(ctx)
		res.Available = ok
		if !ok {
			res.Reason = why
			out = append(out, res)
			continue
		}
		rs, err := s.Fetch(ctx, since)
		if err != nil {
			res.Error = err.Error()
			out = append(out, res)
			continue
		}
		res.Fetched = len(rs)
		n, err := readings.InsertBatch(ctx, st.DB(), rs)
		if err != nil {
			res.Error = err.Error()
		}
		res.Inserted = n
		out = append(out, res)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out
}

func newCollectCmd(flags *rootFlags) *cobra.Command {
	var since time.Duration
	cmd := &cobra.Command{
		Use:     "collect",
		Aliases: []string{"pull", "refresh"},
		Short:   "Pull the latest readings from every sensor into the local history",
		Long: "Fetches current readings from AirThings, IQAir/AirVisual and MOCREO and stores them in\n" +
			"the local SQLite history that `dashboard` reads. MOCREO returns true history (use --since\n" +
			"to backfill); AirThings and IQAir return only their latest snapshot, so long-term history\n" +
			"for those accrues by collecting regularly (e.g. a cron). Re-running is idempotent.",
		Example: "  home-health-pp-cli collect\n  home-health-pp-cli collect --since 168h   # backfill a week of MOCREO\n  home-health-pp-cli collect --json",
		Annotations: map[string]string{
			// Reads vendor APIs and writes only the local cache DB; no external mutation.
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			// collect dials out to every vendor API; under the verifier's mock
			// env, short-circuit instead of making real network calls.
			if cliutil.IsVerifyEnv() {
				fmt.Fprintln(cmd.OutOrStdout(), "would collect latest readings from configured sources (verify mode)")
				return nil
			}
			st, err := openReadings(ctx)
			if err != nil {
				return err
			}
			defer st.Close()
			results := collectAll(ctx, st, time.Now().Add(-since))

			if flags.asJSON || flags.agent {
				return printJSONFiltered(cmd.OutOrStdout(), results, flags)
			}
			out := cmd.OutOrStdout()
			for _, r := range results {
				switch {
				case !r.Available:
					fmt.Fprintf(out, "  %-10s — skipped (%s)\n", r.Source, r.Reason)
				case r.Error != "":
					fmt.Fprintf(out, "  %-10s — ERROR: %s\n", r.Source, r.Error)
				default:
					fmt.Fprintf(out, "  %-10s fetched %d, stored %d new\n", r.Source, r.Fetched, r.Inserted)
				}
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&since, "since", 26*time.Hour,
		"How far back to request (MOCREO honors history; AirThings/IQAir return latest regardless)")
	return cmd
}

package cli

import (
	"github.com/spf13/cobra"
)

func (a *App) questionsCmd() *cobra.Command {
	var sort string
	cmd := &cobra.Command{
		Use:   "questions",
		Short: "List questions from CS Theory Stack Exchange",
		Long: `Fetch questions from cstheory.stackexchange.com via the Stack Exchange API.

Sort options:
  votes     - highest-voted questions first (default)
  activity  - most recently active questions first
  creation  - newest questions first`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			n := a.effectiveLimit(10)
			a.progressf("fetching %d questions (sort=%s)...", n, sort)
			qs, err := a.client.Questions(cmd.Context(), sort, n)
			if err != nil {
				return mapFetchErr(err)
			}
			return a.renderOrEmpty(qs, len(qs))
		},
	}
	cmd.Flags().StringVar(&sort, "sort", "votes", "sort order: votes|activity|creation")
	return cmd
}

package cli

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/wckdboy/wckd-gpu/cli/internal/config"
	"github.com/wckdboy/wckd-gpu/cli/internal/projects"
	"github.com/wckdboy/wckd-gpu/cli/internal/session"
)

type projectView struct {
	ID       string `json:"id"`
	Saved    bool   `json:"saved"`
	Sessions int    `json:"sessions"`
}

func (a *App) projectsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List local project ids",
		Long: `Lists project ids saved on this machine and ids already used by local sessions.

An id maps to the S3 prefix projects/<id>/. Adding an id does not call S3.
wckd start still writes the prefix on the bucket.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := a.open()
			if err != nil {
				return err
			}
			views, err := loadProjectViews(d.cfg.StateDir)
			if err != nil {
				return err
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				return writeJSON(a.stdout(), views)
			}
			writeProjects(a.stdout(), views)
			return nil
		},
	}
	cmd.PersistentFlags().Bool("json", false, "print projects as JSON")
	cmd.AddCommand(a.projectsAddCmd())
	return cmd
}

func (a *App) projectsAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <id>",
		Short: "Remember a local project id",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.ValidateProjectID(args[0]); err != nil {
				return err
			}
			d, err := a.open()
			if err != nil {
				return err
			}
			if _, _, err := projects.Add(d.cfg.StateDir, args[0], time.Now()); err != nil {
				return err
			}
			views, err := loadProjectViews(d.cfg.StateDir)
			if err != nil {
				return err
			}
			var view projectView
			for _, item := range views {
				if item.ID == args[0] {
					view = item
					break
				}
			}
			asJSON, _ := cmd.Flags().GetBool("json")
			if asJSON {
				return writeJSON(a.stdout(), view)
			}
			fmt.Fprintf(a.stdout(), "project: %s\n", view.ID)
			return nil
		},
	}
}

func loadProjectViews(stateDir string) ([]projectView, error) {
	saved, err := projects.Load(stateDir)
	if err != nil {
		return nil, err
	}
	sessions, err := session.NewStore(stateDir).List()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, sess := range sessions {
		if sess.ProjectID == "" {
			continue
		}
		counts[sess.ProjectID]++
	}
	byID := map[string]projectView{}
	for _, rec := range saved {
		byID[rec.ID] = projectView{ID: rec.ID, Saved: true, Sessions: counts[rec.ID]}
	}
	for id, n := range counts {
		view, ok := byID[id]
		if ok {
			view.Sessions = n
			view.Saved = true
			byID[id] = view
			continue
		}
		byID[id] = projectView{ID: id, Saved: false, Sessions: n}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]projectView, 0, len(ids))
	for _, id := range ids {
		out = append(out, byID[id])
	}
	return out, nil
}

func writeProjects(w io.Writer, views []projectView) {
	if len(views) == 0 {
		fmt.Fprintln(w, "no projects")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSAVED\tSESSIONS")
	for _, view := range views {
		saved := "no"
		if view.Saved {
			saved = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\n", view.ID, saved, view.Sessions)
	}
	_ = tw.Flush()
}

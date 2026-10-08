// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// cmd_git.go: `mxcli git note` writes Studio Pro's mx_metadata note for
// commits made outside Studio Pro (mendixlabs/mxcli#1337). See gitnote.go.

var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Keep git commits made outside Studio Pro the way Studio Pro expects",
}

var gitNoteCmd = &cobra.Command{
	Use:   "note [<commit>...]",
	Short: "Write Studio Pro's mx_metadata note for commits made with git",
	Long: `Compute (and with --write, attach) the mx_metadata git note for commits.

Studio Pro attaches a note under refs/notes/mx_metadata to every commit it
makes: the Mendix version and the model units the commit changed. It reads
the note to check version compatibility and to show what a revision changed.
A commit made with plain git has none, and Studio Pro 11.13 back-fills it with
a placeholder ("(unknown)", no changes).

The note is computed from the commit itself — the .mpr unit index and the
mprcontents units at the commit and at its parent — so run it after
committing. Without arguments it covers the commits not yet pushed
(@{upstream}..HEAD), or HEAD when the branch has no upstream.

A commit that already has a real note (one Studio Pro wrote) is left alone;
--force replaces it. A placeholder is replaced.

git push does not push notes. Push the notes ref together with the branch,
before Studio Pro fetches — its fetch force-replaces the local notes ref with
the Team Server's copy and discards notes that were not pushed:

  git push origin <branch> refs/notes/mx_metadata

MPR v2 projects only (Mendix 10.18 and later).`,
	Example: `  mxcli git note -p app.mpr                 # preview notes for unpushed commits
  mxcli git note -p app.mpr --write         # attach them
  mxcli git note -p app.mpr HEAD~2..HEAD --write`,
	RunE:          runGitNote,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	gitNoteCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
	_ = gitNoteCmd.MarkFlagRequired("project")
	gitNoteCmd.Flags().Bool("write", false, "attach the notes (default: print them)")
	gitNoteCmd.Flags().Bool("force", false, "also replace notes Studio Pro wrote")
	gitNoteCmd.Flags().String("branch", "", "BranchName to record (default: the checked-out branch; \"\" on main)")
	gitCmd.AddCommand(gitNoteCmd)
	rootCmd.AddCommand(gitCmd)
}

func runGitNote(cmd *cobra.Command, args []string) error {
	project, _ := cmd.Flags().GetString("project")
	write, _ := cmd.Flags().GetBool("write")
	force, _ := cmd.Flags().GetBool("force")
	out := cmd.OutOrStdout()

	repo, err := openGitNoteRepo(project)
	if err != nil {
		return err
	}
	defer repo.Close()

	branch, _ := cmd.Flags().GetString("branch")
	if !cmd.Flags().Changed("branch") {
		b, _ := gitOut(repo.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
		branch = strings.TrimSpace(b)
	}

	commits, err := gitNoteCommits(repo.Root, args)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		fmt.Fprintln(out, "No commits to annotate: everything on this branch is pushed.")
		return nil
	}
	return annotateCommits(out, repo, commits, branch, write, force)
}

// gitNoteCommits resolves args (commits or ranges) to commit ids, oldest first.
func gitNoteCommits(root string, args []string) ([]string, error) {
	if len(args) == 0 {
		if _, err := gitOut(root, "rev-parse", "--verify", "--quiet", "@{upstream}"); err == nil {
			args = []string{"@{upstream}..HEAD"}
		} else {
			args = []string{"HEAD"}
		}
	}
	var commits []string
	seen := map[string]bool{}
	for _, a := range args {
		var list string
		var err error
		if strings.Contains(a, "..") {
			list, err = gitOut(root, "rev-list", "--reverse", a)
		} else {
			list, err = gitOut(root, "rev-parse", "--verify", a+"^{commit}")
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", a, err)
		}
		for _, c := range strings.Fields(list) {
			if !seen[c] {
				seen[c] = true
				commits = append(commits, c)
			}
		}
	}
	return commits, nil
}

func annotateCommits(out io.Writer, repo *gitNoteRepo, commits []string, branch string, write, force bool) error {
	wrote := 0
	for _, c := range commits {
		subject, _ := gitOut(repo.Root, "log", "-1", "--format=%s", c)
		label := fmt.Sprintf("%s %s", c[:min(len(c), 10)], strings.TrimSpace(subject))
		if existing := repo.existingNote(c); !force && !isPlaceholderNote(existing) {
			fmt.Fprintf(out, "%s\n  kept: already has a Studio Pro note (--force replaces it)\n", label)
			continue
		}
		note, err := repo.buildNote(c, branch)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		text := note.marshal()
		fmt.Fprintf(out, "%s\n  %d model change(s)\n  %s\n", label, len(note.ModelChanges), text)
		if write {
			if err := repo.writeNote(c, text); err != nil {
				return fmt.Errorf("%s: write note: %w", label, err)
			}
			wrote++
		}
	}
	switch {
	case write && wrote > 0:
		target := branch
		if target == "" {
			target = "<branch>"
		}
		fmt.Fprintf(out, "\nWrote %d note(s) to %s. Push them with the branch before Studio Pro fetches:\n  git push origin %s %s\n",
			wrote, mxMetadataRef, target, mxMetadataRef)
	case !write:
		fmt.Fprintln(out, "\nPreview only; --write attaches the notes.")
	}
	return nil
}

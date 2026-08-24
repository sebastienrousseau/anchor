// SPDX-FileCopyrightText: 2026 Sebastien Rousseau <sebastian.rousseau@gmail.com>
// SPDX-License-Identifier: Apache-2.0 OR MIT

package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/sebastienrousseau/anchor/internal/catalog"
	"github.com/sebastienrousseau/anchor/internal/importer"
	"github.com/sebastienrousseau/anchor/internal/registry"
	"github.com/spf13/cobra"
)

// Anchor does not download specifications on the user's behalf. The
// Registration Authority publishes them under terms the person downloading
// accepts, and a tool that clicks through that on their behalf is a tool that
// accepts terms nobody read.
//
// What Anchor can do is remove every other step: find the right message set,
// open the exact download page, then watch for the file to land and import it.
// The user does the part that is theirs to do, and nothing else.

// pollInterval is how often the watch directory is checked. An archive has to
// hold its size across two of these before it is imported.
const pollInterval = time.Second

var (
	fetchWatchDir string
	fetchTimeout  time.Duration
	fetchNoOpen   bool
	fetchDest     string
)

var catalogFetchCmd = &cobra.Command{
	Use:   "fetch <message-or-set>",
	Short: "Find a message set, open its download page, and import it when it arrives",
	Long: `Fetch takes the manual steps out of installing a message set, without
downloading anything on your behalf.

Given a message identifier or a set name it finds the right message set, opens
the Registration Authority's download page for it, then watches your downloads
folder. When the archive appears it is imported and verified, and Anchor tells
you what it installed.

Anchor does not download the archive itself. The Registration Authority
publishes these files under terms the person downloading them accepts, and a
tool that clicks through those terms is a tool that accepts them for someone who
never read them.`,
	Example: `  anchor catalog fetch pacs.008
  anchor catalog fetch "Payments Clearing and Settlement"
  anchor catalog fetch camt.053 --watch ~/Downloads --timeout 10m
  anchor catalog fetch pacs.008 --no-open`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := registry.Load()
		if err != nil {
			return err
		}

		sets := matchSets(reg, args[0])
		if len(sets) == 0 {
			return fmt.Errorf("nothing in the published standard matches %q\n\n"+
				"Try a message identifier (pacs.008), a domain (camt), or a set name "+
				"(\"Payments Clearing and Settlement\").\nList everything: anchor catalog status --all", args[0])
		}
		if len(sets) > 1 {
			return ambiguousSets(args[0], sets)
		}
		set := sets[0]

		watchDir, err := resolveWatchDir()
		if err != nil {
			return err
		}

		fmt.Printf("\n%s %s\n\n", headStyle.Render(" FETCH "), titleStyle.Render(set.String()))
		fmt.Printf("  %-10s %s\n", "set", set.Name)
		fmt.Printf("  %-10s %s\n", "version", set.Version)
		fmt.Printf("  %-10s %d\n", "files", set.FilesCount)
		fmt.Printf("  %-10s %s\n\n", "download", set.DownloadURL())

		if !fetchNoOpen {
			if err := openInBrowser(set.DownloadURL()); err != nil {
				fmt.Printf("  %s could not open a browser: %v\n", warnMark, err)
				fmt.Printf("  Open the link above yourself.\n\n")
			}
		}

		fmt.Printf("  %s Accept the Registration Authority's terms and download the archive.\n",
			subtleStyle.Render("1."))
		fmt.Printf("  %s Anchor is watching %s and will import it when it lands.\n\n",
			subtleStyle.Render("2."), watchDir)
		fmt.Printf("  waiting up to %s   (ctrl-c to stop; you can always run: anchor catalog add <file>)\n\n",
			fetchTimeout)

		archive, err := waitForArchive(watchDir, fetchTimeout)
		if err != nil {
			return err
		}
		fmt.Printf("  %s found %s\n\n", tickMark, archive)

		return importFetched(archive, set)
	},
}

// matchSets finds the message sets an identifier or name refers to.
func matchSets(reg *registry.Registry, query string) []registry.Set {
	q := strings.ToLower(strings.TrimSpace(query))

	// A message identifier names the sets that publish it, which is the common
	// case: someone knows they need pacs.008, not which set carries it.
	if hits := reg.Search(q); len(hits) > 0 {
		byID := map[string]registry.Set{}
		for _, m := range hits {
			for _, s := range reg.SetsFor(m.ID) {
				byID[s.ID] = s
			}
		}
		if len(byID) > 0 {
			return latestPerName(byID)
		}
	}

	// Otherwise match the set names.
	byID := map[string]registry.Set{}
	for _, s := range reg.Sets {
		if strings.Contains(strings.ToLower(s.Name), q) {
			byID[s.ID] = s
		}
	}
	return latestPerName(byID)
}

// latestPerName keeps the newest version of each named set: a user asking for
// "Payments Clearing and Settlement" wants the current one, not fourteen.
func latestPerName(sets map[string]registry.Set) []registry.Set {
	newest := map[string]registry.Set{}
	for _, s := range sets {
		if got, ok := newest[s.Name]; !ok || s.Version > got.Version {
			newest[s.Name] = s
		}
	}

	out := make([]registry.Set, 0, len(newest))
	for _, s := range newest {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func ambiguousSets(query string, sets []registry.Set) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%q is published in %d message sets:\n\n", query, len(sets))
	for _, s := range sets {
		fmt.Fprintf(&b, "  %-52s %s\n", s.Name, s.Version)
	}
	b.WriteString("\nName the one you want.")
	return errors.New(b.String())
}

// resolveWatchDir picks the directory to watch for the download.
func resolveWatchDir() (string, error) {
	if fetchWatchDir != "" {
		dir := expandHome(fetchWatchDir)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return "", fmt.Errorf("not a directory: %s", dir)
		}
		return dir, nil
	}

	home := catalog.HomeDir()
	if home == "" {
		return "", errors.New("could not find your home directory; pass --watch")
	}
	downloads := filepath.Join(home, "Downloads")
	if info, err := os.Stat(downloads); err == nil && info.IsDir() {
		return downloads, nil
	}
	return "", fmt.Errorf("no Downloads folder at %s; pass --watch <directory>", downloads)
}

func expandHome(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home := catalog.HomeDir()
	if home == "" {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}

// waitForArchive polls a directory until a zip appears that was not there when
// the wait began.
//
// Polling rather than watching is deliberate: a browser writes a download under
// a temporary name and renames it, and a file-system watcher reports the
// temporary file as readily as the finished one. Polling for a settled size
// avoids importing a half-written archive.
func waitForArchive(dir string, timeout time.Duration) (string, error) {
	before, err := zipsIn(dir)
	if err != nil {
		return "", err
	}

	// pending holds archives that appeared after the wait began, with the size
	// last seen. An archive is only imported once its size has stopped changing
	// between two polls, so a download still being written is never touched.
	pending := map[string]int64{}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)

		now, err := zipsIn(dir)
		if err != nil {
			return "", err
		}
		for path, size := range now {
			if _, existed := before[path]; existed {
				continue
			}
			if last, seen := pending[path]; seen && last == size && size > 0 {
				return path, nil
			}
			pending[path] = size
		}
	}

	return "", fmt.Errorf("no new archive appeared in %s within %s\n\n"+
		"Download it yourself, then run:\n  anchor catalog add <downloaded.zip>", dir, timeout)
}

func zipsIn(dir string) (map[string]int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}

	out := map[string]int64{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out[filepath.Join(dir, e.Name())] = info.Size()
	}
	return out, nil
}

// importFetched installs the archive and reports what landed.
func importFetched(archive string, set registry.Set) error {
	dest := fetchDest
	if dest == "" {
		dest = catalogPath
	}
	if dest == "" {
		dest = os.Getenv(catalog.EnvCatalog)
	}
	if dest == "" {
		dest = catalog.DefaultDir()
	}

	res, err := importer.ImportArchive(archive, importer.Options{
		Root:     dest,
		Category: set.Name,
		Limits:   importer.DefaultLimits(),
	})
	if err != nil {
		return fmt.Errorf("importing %s: %w\n\nImport it yourself with: anchor catalog add %s",
			filepath.Base(archive), err, archive)
	}

	fmt.Printf("%s %s\n\n", headStyle.Render(" INSTALLED "), titleStyle.Render(set.Name))
	fmt.Printf("  %-10s %s\n", "into", dest)
	fmt.Printf("  %-10s %d\n", "schemas", res.Schemas)
	fmt.Printf("  %-10s %d\n", "samples", res.Samples)
	fmt.Printf("  %-10s %d\n", "reports", res.Reports+res.Guidelines+res.Docs)
	fmt.Printf("  %-10s %s\n\n", "written", humanBytes(res.BytesWritten))

	fmt.Printf("  %s anchor stats\n", subtleStyle.Render("→"))
	fmt.Printf("  %s anchor catalog status\n\n", subtleStyle.Render("→"))
	return nil
}

// humanBytes renders a size the way a reader thinks about one.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

// openInBrowser opens a URL with the platform's own handler. Nothing is
// downloaded here; the browser is where the user accepts the terms.
func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func init() {
	catalogFetchCmd.Flags().StringVar(&fetchWatchDir, "watch", "",
		"Directory to watch for the download (default: your Downloads folder)")
	catalogFetchCmd.Flags().DurationVar(&fetchTimeout, "timeout", 5*time.Minute,
		"How long to wait for the archive to appear")
	catalogFetchCmd.Flags().BoolVar(&fetchNoOpen, "no-open", false,
		"Print the download link instead of opening a browser")
	catalogFetchCmd.Flags().StringVar(&fetchDest, "to", "",
		"Catalogue root to install into")
	catalogCmd.AddCommand(catalogFetchCmd)
}

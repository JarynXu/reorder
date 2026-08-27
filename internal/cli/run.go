package cli

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JarynXu/reorder"
)

// Run executes the reorder command and returns a process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, version string) int {
	flags := flag.NewFlagSet("reorder", flag.ContinueOnError)
	flags.SetOutput(stderr)

	cfg := reorder.DefaultConfig()
	flags.BoolVar(&cfg.Constructor, "constructor", cfg.Constructor, "place constructors after their declared type and before its methods")
	flags.BoolVar(&cfg.StructMethod, "struct-method", cfg.StructMethod, "place exported methods before unexported methods")
	flags.BoolVar(&cfg.Alphabetical, "alphabetical", cfg.Alphabetical, "sort enabled constructor/method groups alphabetically")
	flags.BoolVar(&cfg.Function, "function", cfg.Function, "place exported top-level functions before unexported functions (excluding init)")

	write := flags.Bool("write", false, "rewrite files in place")
	check := flags.Bool("check", false, "list files that would change and exit 1 when changes are needed")
	showVersion := flags.Bool("version", false, "print version and exit")
	stdinFilename := flags.String("stdin-filename", "stdin.go", "filename used for parsing stdin and diagnostics")

	flags.Usage = func() {
		fmt.Fprintf(stderr, "Usage: reorder [flags] [file.go|directory|...]\n\n")
		fmt.Fprintf(stderr, "With no path (or with '-'), reorder reads Go source from stdin and writes the reordered source to stdout.\n")
		fmt.Fprintf(stderr, "For file/directory targets, use -write to modify files or -check for CI.\n\nFlags:\n")
		flags.PrintDefaults()
	}

	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if *write && *check {
		fmt.Fprintln(stderr, "reorder: -write and -check cannot be used together")
		return 2
	}

	targets := flags.Args()
	if len(targets) == 0 || (len(targets) == 1 && targets[0] == "-") {
		if *write {
			fmt.Fprintln(stderr, "reorder: -write cannot be used with stdin")
			return 2
		}
		return runStdin(stdin, stdout, stderr, *stdinFilename, cfg, *check)
	}
	for _, target := range targets {
		if target == "-" {
			fmt.Fprintln(stderr, "reorder: stdin '-' cannot be combined with file targets")
			return 2
		}
	}

	paths, err := discoverGoFiles(targets)
	if err != nil {
		fmt.Fprintf(stderr, "reorder: %v\n", err)
		return 2
	}
	if len(paths) == 0 {
		return 0
	}
	if !*write && !*check && len(paths) != 1 {
		fmt.Fprintln(stderr, "reorder: multiple file targets require -write or -check")
		return 2
	}

	changes, err := planFiles(paths, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "reorder: %v\n", err)
		return 2
	}

	if *check {
		for _, change := range changes {
			if change.changed {
				fmt.Fprintln(stdout, change.path)
			}
		}
		for _, change := range changes {
			if change.changed {
				return 1
			}
		}
		return 0
	}

	if *write {
		for _, change := range changes {
			if !change.changed {
				continue
			}
			if err := atomicWriteFile(change.path, change.output, change.mode); err != nil {
				fmt.Fprintf(stderr, "reorder: write %s: %v\n", change.path, err)
				return 2
			}
		}
		return 0
	}

	_, _ = stdout.Write(changes[0].output)
	return 0
}

func runStdin(stdin io.Reader, stdout, stderr io.Writer, filename string, cfg reorder.Config, check bool) int {
	src, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "reorder: read stdin: %v\n", err)
		return 2
	}
	out, changed, err := reorder.Rewrite(filename, src, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "reorder: %v\n", err)
		return 2
	}
	if check {
		if changed {
			return 1
		}
		return 0
	}
	_, _ = stdout.Write(out)
	return 0
}

type fileChange struct {
	path    string
	output  []byte
	mode    fs.FileMode
	changed bool
}

func planFiles(paths []string, cfg reorder.Config) ([]fileChange, error) {
	changes := make([]fileChange, 0, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to rewrite symlink %s", path)
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		out, changed, err := reorder.Rewrite(path, src, cfg)
		if err != nil {
			return nil, err
		}
		changes = append(changes, fileChange{path: path, output: out, mode: info.Mode(), changed: changed})
	}
	return changes, nil
}

func discoverGoFiles(targets []string) ([]string, error) {
	seen := make(map[string]struct{})
	var paths []string

	for _, target := range targets {
		walkTarget := target
		if strings.HasSuffix(target, "...") {
			walkTarget = strings.TrimSuffix(target, "...")
			walkTarget = strings.TrimSuffix(walkTarget, string(filepath.Separator))
			if walkTarget == "" {
				walkTarget = "."
			}
		}

		info, err := os.Stat(walkTarget)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", target, err)
		}
		if !info.IsDir() {
			if filepath.Ext(walkTarget) != ".go" {
				return nil, fmt.Errorf("not a Go source file: %s", walkTarget)
			}
			addPath(&paths, seen, walkTarget)
			continue
		}

		err = filepath.WalkDir(walkTarget, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if path != walkTarget && (entry.Name() == ".git" || entry.Name() == "vendor") {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".go" {
				return nil
			}
			addPath(&paths, seen, path)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", walkTarget, err)
		}
	}

	sort.Strings(paths)
	return paths, nil
}

func addPath(paths *[]string, seen map[string]struct{}, path string) {
	clean := filepath.Clean(path)
	if _, ok := seen[clean]; ok {
		return
	}
	seen[clean] = struct{}{}
	*paths = append(*paths, clean)
}

func atomicWriteFile(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".reorder-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}

	if err := temp.Chmod(mode.Perm()); err != nil {
		cleanup()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		_ = os.Remove(tempName)
		return err
	}
	return nil
}

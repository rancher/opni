package testutil

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	glob "github.com/bmatcuk/doublestar/v4"
	"golang.org/x/tools/cover"
)

type ProfileOptions struct {
	excludePatterns    []string
	keepMergedProfiles bool
}

type ProfileOption func(*ProfileOptions)

func (o *ProfileOptions) apply(opts ...ProfileOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithExcludePatterns(excludePatterns ...string) ProfileOption {
	return func(o *ProfileOptions) {
		o.excludePatterns = excludePatterns
	}
}

func WithKeepMergedProfiles(keep bool) ProfileOption {
	return func(o *ProfileOptions) {
		o.keepMergedProfiles = keep
	}
}

func MergeCoverProfiles(filenames []string, output io.Writer, opts ...ProfileOption) error {
	options := ProfileOptions{}
	options.apply(opts...)
	// read contents of all files into a single buffer, keeping only one mode line
	mode := ""
	buf := new(bytes.Buffer)
	for _, filename := range filenames {
		f, err := os.Open(filename)
		if err != nil {
			return err
		}
		defer func() {
			f.Close()
			if !options.keepMergedProfiles {
				os.Remove(f.Name())
			}
		}()
		s := bufio.NewScanner(f)
		s.Scan()
		if mode == "" {
			modeLine := s.Text()
			if !strings.HasPrefix(modeLine, "mode: ") {
				return fmt.Errorf("invalid mode line: %s", modeLine)
			}
			mode = modeLine[6:]
			if _, err := fmt.Fprintln(buf, modeLine); err != nil {
				return err
			}
		}
		for s.Scan() {
			if _, err := fmt.Fprintln(buf, s.Text()); err != nil {
				return err
			}
		}
		if err := s.Err(); err != nil {
			return err
		}
	}
	if mode == "" {
		return fmt.Errorf("no mode found")
	}

	// this merges all the profile blocks
	profiles, err := cover.ParseProfilesFromReader(buf)
	if err != nil {
		return err
	}

	// write the merged profile
	if _, err := fmt.Fprintf(output, "mode: %s\n", mode); err != nil {
		return err
	}
	var active, total int64
PROFILE:
	for _, profile := range profiles {
		for _, pattern := range options.excludePatterns {
			matched, err := glob.PathMatch(pattern, profile.FileName)
			if err != nil {
				return err
			}
			if matched {
				continue PROFILE
			}
		}
		for _, block := range profile.Blocks {
			stmts := int64(block.NumStmt)
			total += stmts
			if block.Count > 0 {
				active += stmts
			}
			if _, err := fmt.Fprintf(output, "%s:%d.%d,%d.%d %d %d\n", profile.FileName,
				block.StartLine, block.StartCol,
				block.EndLine, block.EndCol,
				stmts,
				block.Count,
			); err != nil {
				return err
			}
		}
	}
	if total == 0 {
		return nil
	}

	fmt.Printf("combined coverage from %d profiles: %.1f%% of statements\n", len(filenames), 100*float64(active)/float64(total))
	return nil
}

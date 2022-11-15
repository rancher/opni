package testutil

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/samber/lo"

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

type pkgCoverage struct {
	pkg         string
	coverage    float64
	sourceLines int
	sourceFiles int
	statements  int64
	covered     int64
}

func GenerateCoverageSummary(filename string) (string, error) {
	profiles, err := cover.ParseProfiles(filename)
	if err != nil {
		return "", err
	}
	var active, total int64
	pkgActive := make(map[string]int64)
	pkgTotal := make(map[string]int64)
	for _, profile := range profiles {
		pkg := filepath.Dir(profile.FileName)
		for _, block := range profile.Blocks {
			stmts := int64(block.NumStmt)
			total += stmts
			pkgTotal[pkg] += stmts
			if block.Count > 0 {
				active += stmts
				pkgActive[pkg] += stmts
			}
		}
	}

	entries := []pkgCoverage{}
	var locTotal, filesTotal int
	for pkg, total := range pkgTotal {
		// compute lines of code
		loc := 0
		dirname := strings.Replace(pkg, "github.com/rancher/opni/", "", 1)
		fileCount := 0
		for _, f := range lo.Must(os.ReadDir(dirname)) {
			if f.IsDir() {
				continue
			}
			path := filepath.Join(dirname, f.Name())
			if !strings.HasSuffix(f.Name(), ".go") {
				continue
			}
			if strings.HasSuffix(f.Name(), "_test.go") {
				continue
			}
			f, err := os.Open(path)
			if err != nil {
				continue
			}
			defer f.Close()
			fileCount++
			s := bufio.NewScanner(f)
			for s.Scan() {
				loc++
			}
		}
		locTotal += loc
		filesTotal += fileCount

		info := pkgCoverage{
			pkg:         pkg,
			coverage:    100 * float64(pkgActive[pkg]) / float64(total),
			sourceFiles: fileCount,
			sourceLines: loc,
			statements:  total,
			covered:     pkgActive[pkg],
		}
		entries = append(entries, info)
	}
	sort.Slice(entries, func(i, j int) bool {
		coverI := entries[i].coverage
		coverJ := entries[j].coverage
		if coverI == coverJ {
			return entries[i].pkg < entries[j].pkg
		}
		return coverI > coverJ
	})

	// Print packages by coverage

	resultBuffer := strings.Builder{}
	t := table.NewWriter()
	t.AppendHeader(table.Row{"Package", "Coverage", "Files", "Lines", "Statements", "Covered"})
	for _, entry := range entries {
		t.AppendRow(table.Row{entry.pkg, fmt.Sprintf("%.1f%%", entry.coverage), entry.sourceFiles, entry.sourceLines, entry.statements, entry.covered})
	}
	t.AppendFooter(table.Row{"Total", fmt.Sprintf("%.1f%%", 100*float64(active)/float64(total)), filesTotal, locTotal, total, active})
	t.SetStyle(table.StyleLight)
	resultBuffer.WriteString(t.Render() + "\n")

	numPackagesByThreshold := map[int]int{}
	numFilesByThreshold := map[int]int{}
	locByThreshold := map[int]int{}
	for _, e := range entries {
		switch {
		case e.coverage >= 80:
			numPackagesByThreshold[80]++
			numFilesByThreshold[80] += e.sourceFiles
			locByThreshold[80] += e.sourceLines
		case e.coverage >= 60:
			numPackagesByThreshold[60]++
			numFilesByThreshold[60] += e.sourceFiles
			locByThreshold[60] += e.sourceLines
		case e.coverage >= 40:
			numPackagesByThreshold[40]++
			numFilesByThreshold[40] += e.sourceFiles
			locByThreshold[40] += e.sourceLines
		case e.coverage >= 20:
			numPackagesByThreshold[20]++
			numFilesByThreshold[20] += e.sourceFiles
			locByThreshold[20] += e.sourceLines
		case e.coverage > 0:
			numPackagesByThreshold[1]++
			numFilesByThreshold[1] += e.sourceFiles
			locByThreshold[1] += e.sourceLines
		case e.coverage == 0:
			numPackagesByThreshold[0]++
			numFilesByThreshold[0] += e.sourceFiles
			locByThreshold[0] += e.sourceLines
		}
	}

	resultBuffer.WriteString(fmt.Sprintf(
		"%d packages with no tests (%d files, %d lines)\n", numPackagesByThreshold[0], numFilesByThreshold[0], locByThreshold[0]))
	resultBuffer.WriteString(fmt.Sprintf(
		"%d packages with less than 20%% coverage (%d files, %d lines)\n", numPackagesByThreshold[1], numFilesByThreshold[1], locByThreshold[1]))
	resultBuffer.WriteString(fmt.Sprintf(
		"%d packages with less than 40%% coverage (%d files, %d lines)\n", numPackagesByThreshold[20], numFilesByThreshold[20], locByThreshold[20]))
	resultBuffer.WriteString(fmt.Sprintf(
		"%d packages with less than 60%% coverage (%d files, %d lines)\n", numPackagesByThreshold[40], numFilesByThreshold[40], locByThreshold[40]))
	resultBuffer.WriteString(fmt.Sprintf(
		"%d packages with less than 80%% coverage (%d files, %d lines)\n", numPackagesByThreshold[60], numFilesByThreshold[60], locByThreshold[60]))
	resultBuffer.WriteString(fmt.Sprintf(
		"%d packages with 80%% or more coverage (%d files, %d lines)\n", numPackagesByThreshold[80], numFilesByThreshold[80], locByThreshold[80]))

	return resultBuffer.String(), nil
}

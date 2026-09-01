// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command changelog creates changelog fragment files under changelog/.
//
// Fragments are collected into CHANGELOG.md at release time by
// hack/update-changelog.sh and validated in CI by hack/changelog-check.sh.
// See changelog/README.md for the file format.
//
// Usage:
//
//	go tool changelog generate
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	fragmentDir  = "changelog"
	issueURLBase = "https://github.com/istio-ecosystem/sail-operator/issues/"
	githubPrefix = "https://github.com/"

	// descriptionWidth is the total column width of a wrapped description line,
	// including the two-space YAML block indent.
	descriptionWidth = 78
	blockIndent      = "  "
	maxSlugLen       = 50
)

// categories are the valid values of the fragment's "category" field, in the
// order hack/update-changelog.sh renders them.
var categories = []string{"added", "changed", "fixed", "removed"}

// errAborted signals that the user ended input without completing the fragment.
var errAborted = errors.New("aborted")

func main() {
	if len(os.Args) != 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "generate":
		if err := generate(); err != nil {
			if errors.Is(err, errAborted) {
				fmt.Fprintln(os.Stderr, "aborted, no file written")
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `changelog - create a changelog fragment for this pull request

Usage:
  go tool changelog generate    Prompt for the fragment fields and write changelog/<slug>.yaml
  go tool changelog help        Show this message
`)
}

func generate() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}

	p := &prompter{r: bufio.NewReader(os.Stdin)}
	fmt.Println("Creating a changelog fragment. See changelog/README.md for the format.")

	category, err := p.category()
	if err != nil {
		return err
	}

	title, err := p.required("Title (one line, e.g. \"Fix ZTunnel DNS config being dropped\")")
	if err != nil {
		return err
	}
	if needsQuoting(title) {
		fmt.Fprint(os.Stderr, "  note: this title needs YAML quoting, and hack/update-changelog.sh reads the\n"+
			"  field with grep, so the quotes will show up in CHANGELOG.md. Consider\n"+
			"  rewording to avoid \": \" and leading punctuation.\n")
	}

	description, err := p.multiline("Description (optional, blank line to finish):")
	if err != nil {
		return err
	}

	issueLink, err := p.issueLink(category)
	if err != nil {
		return err
	}

	slug := slugify(title)
	if !isValidSlug(slug) {
		slug = category + "-entry"
	}

	path := filepath.Join(root, fragmentDir, slug+".yaml")
	display := filepath.Join(fragmentDir, slug+".yaml")
	switch _, err := os.Stat(path); {
	case err == nil:
		overwrite, err := p.confirm(fmt.Sprintf("%s already exists. Overwrite?", display))
		if err != nil {
			return err
		}
		if !overwrite {
			return errAborted
		}
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("checking %s: %w", display, err)
	}

	content := render(category, title, description, issueLink)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", display, err)
	}

	fmt.Printf("\nWrote %s\n\n%s\n", display, content)
	return nil
}

// render builds the fragment YAML. Field order matches changelog/README.md.
func render(category, title, description, issueLink string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "category: %s\n", category)
	fmt.Fprintf(&b, "title: %s\n", yamlScalar(title))
	if description != "" {
		b.WriteString("description: |\n")
		for _, line := range wrap(description, descriptionWidth-len(blockIndent)) {
			b.WriteString(blockIndent + line + "\n")
		}
	}
	if issueLink != "" {
		fmt.Fprintf(&b, "issueLink: %s\n", issueLink)
	}
	return b.String()
}

// repoRoot walks up from the working directory looking for the module root, so
// the tool works both from tools/ and from the repository root.
func repoRoot() (string, error) {
	start, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for dir := start; ; {
		if isDir(filepath.Join(dir, fragmentDir)) && exists(filepath.Join(dir, "go.mod")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no directory containing go.mod and %s/ found at or above %s", fragmentDir, start)
		}
		dir = parent
	}
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// prompter reads answers from stdin. Every read reports errAborted on EOF so a
// Ctrl-D at any prompt leaves the working tree untouched.
type prompter struct {
	r *bufio.Reader
}

func (p *prompter) read() (string, error) {
	fmt.Print("> ")
	line, err := p.r.ReadString('\n')
	line = strings.TrimSpace(line)
	if errors.Is(err, io.EOF) {
		fmt.Println()
		if line == "" {
			return "", errAborted
		}
		return line, nil
	}
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	return line, nil
}

func (p *prompter) category() (string, error) {
	fmt.Println("\nCategory:")
	for i, c := range categories {
		fmt.Printf("  %d) %s\n", i+1, c)
	}

	for {
		in, err := p.read()
		if err != nil {
			return "", err
		}
		in = strings.ToLower(in)
		if n, err := strconv.Atoi(in); err == nil && n >= 1 && n <= len(categories) {
			return categories[n-1], nil
		}
		for _, c := range categories {
			if c == in {
				return c, nil
			}
		}
		fmt.Fprintf(os.Stderr, "  enter 1-%d or one of: %s\n", len(categories), strings.Join(categories, ", "))
	}
}

func (p *prompter) required(label string) (string, error) {
	fmt.Printf("\n%s\n", label)
	for {
		in, err := p.read()
		if err != nil {
			return "", err
		}
		if in != "" {
			return in, nil
		}
		fmt.Fprintln(os.Stderr, "  required")
	}
}

// multiline reads lines until a blank one and joins them into a single
// paragraph; render re-wraps it to the fragment's column width.
func (p *prompter) multiline(label string) (string, error) {
	fmt.Printf("\n%s\n", label)
	var lines []string
	for {
		in, err := p.read()
		if errors.Is(err, errAborted) && len(lines) > 0 {
			break
		}
		if err != nil {
			return "", err
		}
		if in == "" {
			break
		}
		lines = append(lines, in)
	}
	return strings.Join(lines, " "), nil
}

// issueLink enforces the same rules as hack/changelog-check.sh: required for
// "fixed" entries, and a github.com URL when set. A bare issue number is
// expanded to this repository's issue URL.
func (p *prompter) issueLink(category string) (string, error) {
	required := category == "fixed"
	if required {
		fmt.Println("\nIssue link (required for 'fixed'; issue number or GitHub URL):")
	} else {
		fmt.Println("\nIssue link (optional; issue number or GitHub URL, blank to skip):")
	}

	for {
		in, err := p.read()
		if err != nil {
			return "", err
		}
		switch {
		case in == "" && !required:
			return "", nil
		case in == "":
			fmt.Fprintln(os.Stderr, "  required for 'fixed' entries")
		case isNumeric(in):
			return issueURLBase + in, nil
		case strings.HasPrefix(in, githubPrefix):
			return in, nil
		default:
			fmt.Fprintf(os.Stderr, "  enter an issue number or a %s URL\n", githubPrefix)
		}
	}
}

func (p *prompter) confirm(question string) (bool, error) {
	fmt.Printf("\n%s [y/N]\n", question)
	in, err := p.read()
	if errors.Is(err, errAborted) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	in = strings.ToLower(in)
	return in == "y" || in == "yes", nil
}

// slugify derives the fragment's filename stem from the title, truncated at a
// word boundary so the name stays readable.
func slugify(title string) string {
	var b strings.Builder
	dashed := true // suppresses a leading dash
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || unicode.IsDigit(r) {
			b.WriteRune(r)
			dashed = false
			continue
		}
		if !dashed {
			b.WriteByte('-')
			dashed = true
		}
	}

	s := strings.Trim(b.String(), "-")
	if len(s) > maxSlugLen {
		s = s[:maxSlugLen]
		if i := strings.LastIndexByte(s, '-'); i > 0 {
			s = s[:i]
		}
	}
	return s
}

func isValidSlug(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") || strings.HasSuffix(s, "-") {
		return false
	}
	for _, r := range s {
		if r != '-' && !(r >= 'a' && r <= 'z') && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// yamlScalar quotes a value only when it cannot be written as a plain scalar,
// keeping fragments readable and parseable by the grep-based release script.
func yamlScalar(s string) string {
	if needsQuoting(s) {
		return strconv.Quote(s)
	}
	return s
}

func needsQuoting(s string) bool {
	if s == "" || strings.TrimSpace(s) != s {
		return true
	}
	// YAML indicator characters may not start a plain scalar.
	if strings.ContainsAny(s[:1], "-?:,[]{}#&*!|>'\"%@`") {
		return true
	}
	// ": " opens a mapping and " #" opens a comment anywhere in the line.
	return strings.Contains(s, ": ") || strings.HasSuffix(s, ":") || strings.Contains(s, " #")
}

// wrap greedily fills lines of at most width columns.
func wrap(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}

	lines := make([]string, 0, len(words))
	current := words[0]
	for _, w := range words[1:] {
		if utf8.RuneCountInString(current)+1+utf8.RuneCountInString(w) > width {
			lines = append(lines, current)
			current = w
			continue
		}
		current += " " + w
	}
	return append(lines, current)
}

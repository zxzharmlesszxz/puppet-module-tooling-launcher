package pmtool

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func (a *app) colorEnabled() bool {
	if a.getenv("NO_COLOR") != "" {
		return false
	}
	mode := a.color
	if mode == "" {
		mode = a.getenv("TOOLING_COLOR")
	}
	return mode == "always" || ((mode == "auto" || mode == "") && a.tty && a.getenv("TERM") != "dumb")
}

func (a *app) paint(text, style string) string {
	if a.colorEnabled() {
		return "\x1b[" + style + "m" + text + "\x1b[0m"
	}
	return text
}

func (a *app) notice(label, text, style string) {
	must(fmt.Fprintln(a.stderr, a.paint("["+label+"] "+text, style)))
}

var headingPattern = regexp.MustCompile(`^(#{1,6}) (.+)$`)
var listPattern = regexp.MustCompile(`^(\s*(?:[-*]|[0-9]+\.)\s+)(.+)$`)
var linkPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

type heading struct {
	line, level int
	title       string
}

func cleanMarkdown(text string) string {
	text = linkPattern.ReplaceAllString(text, "$1 ($2)")
	text = strings.ReplaceAll(text, "**", "")
	return strings.ReplaceAll(text, "`", "")
}

func wrapText(text string, width int) []string {
	lines := []string{""}
	for word := range strings.FieldsSeq(text) {
		last := len(lines) - 1
		if lines[last] != "" && utf8.RuneCountInString(lines[last])+utf8.RuneCountInString(word)+1 > width {
			lines = append(lines, word)
		} else {
			if lines[last] != "" {
				lines[last] += " "
			}
			lines[last] += word
		}
	}
	return lines
}

func (a *app) document(markdown, section string) string {
	lines := strings.SplitAfter(markdown, "\n")
	var headings []heading
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
		}
		if match := headingPattern.FindStringSubmatch(strings.TrimSpace(line)); !fenced && match != nil {
			headings = append(headings, heading{i, len(match[1]), match[2]})
		}
	}
	if section == "list" {
		var titles []string
		for _, h := range headings {
			titles = append(titles, strings.Repeat("  ", max(h.level-2, 0))+h.title)
		}
		return strings.Join(titles, "\n") + "\n"
	}
	if section != "" {
		var matches []heading
		for _, h := range headings {
			if strings.Contains(strings.ToLower(h.title), strings.ToLower(section)) {
				matches = append(matches, h)
			}
		}
		if len(matches) != 1 {
			fail("section %q: expected one match; use --section list", section)
		}
		selected := matches[0]
		end := len(lines)
		for _, h := range headings {
			if h.line > selected.line && h.level <= selected.level {
				end = h.line
				break
			}
		}
		markdown = strings.Join(lines[selected.line:end], "")
	}
	if !a.tty {
		return markdown
	}
	width := 88
	if a.columns > 0 {
		width = min(max(a.columns, 40), 120)
	}
	if columns, err := strconv.Atoi(a.getenv("COLUMNS")); err == nil {
		width = min(max(columns, 40), 120)
	}
	var result []string
	var paragraph []string
	flush := func() {
		if len(paragraph) > 0 {
			result = append(result, wrapText(cleanMarkdown(strings.Join(paragraph, " ")), width)...)
			paragraph = nil
		}
	}
	fenced = false
	for line := range strings.SplitSeq(strings.TrimSuffix(markdown, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "```"):
			flush()
			fenced = !fenced
			if fenced {
				result = append(result, a.paint("  ── "+strings.TrimPrefix(line, "```")+" ──", "2"))
			} else {
				result = append(result, "")
			}
		case fenced:
			result = append(result, a.paint("  "+line, "36"))
		case headingPattern.MatchString(line):
			flush()
			match := headingPattern.FindStringSubmatch(line)
			result = append(result, "", a.paint(cleanMarkdown(match[2]), "1;33"), "")
		case line == "":
			flush()
			result = append(result, "")
		case listPattern.MatchString(line):
			flush()
			match := listPattern.FindStringSubmatch(line)
			for i, part := range wrapText(cleanMarkdown(match[2]), width-utf8.RuneCountInString(match[1])) {
				prefix := match[1]
				if i > 0 {
					prefix = strings.Repeat(" ", utf8.RuneCountInString(prefix))
				}
				result = append(result, prefix+part)
			}
		case strings.Contains(line, "|"):
			flush()
			result = append(result, line)
		default:
			paragraph = append(paragraph, strings.TrimSpace(line))
		}
	}
	flush()
	return strings.TrimSpace(strings.Join(result, "\n")) + "\n"
}

func (a *app) usage() string {
	lines := strings.Split(strings.TrimSuffix(strings.TrimPrefix(shortHelp, "# "), "\n"), "\n")
	for i, line := range lines {
		if i == 0 || strings.HasPrefix(line, "Usage:") || strings.HasSuffix(line, ":") {
			lines[i] = a.paint(line, "1;33")
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

package display

import (
	"fmt"
	"strings"
)

const (
	Bold   = "\033[1m"
	Dim    = "\033[2m"
	Red    = "\033[91m"
	Green  = "\033[92m"
	Yellow = "\033[93m"
	Cyan   = "\033[96m"
	White  = "\033[97m"
	Reset  = "\033[0m"
)

func Banner() {
	fmt.Printf("\n  %s█▄ █ █▀█ ▀█▀ █▀▀%s\n", Cyan, Reset)
	fmt.Printf("  %s█ ▀█ █▄█  █  ██▄%s\n\n", Cyan, Reset)
}

func Success(msg string) {
	fmt.Printf("  %s%s%s\n", Green, msg, Reset)
}

func Error(msg string) {
	fmt.Printf("  %s%s%s\n", Red, msg, Reset)
}

func Info(msg string) {
	fmt.Printf("  %s%s%s\n", Dim, msg, Reset)
}

func NoteTitle(title string) {
	fmt.Printf("  %s%s%s%s\n", Bold, White, title, Reset)
}

func NoteTags(tags []string) {
	if len(tags) > 0 {
		fmt.Printf("  %s", Dim)
		for i, t := range tags {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			if i > 0 {
				fmt.Print("  ")
			}
			fmt.Printf("%s#%s%s", Cyan, t, Reset+Dim)
		}
		fmt.Printf("%s\n", Reset)
	}
}

func SearchResult(file string, line int, content string, query string) {
	name := strings.TrimSuffix(file, ".md")
	fmt.Printf("\n  %s╭─%s %s%s%s (line %d)\n", Cyan, Reset, Bold, name, Reset, line)

	// Highlight matching terms
	terms := strings.Fields(strings.ToLower(query))
	highlighted := content
	for _, term := range terms {
		idx := strings.Index(strings.ToLower(highlighted), term)
		if idx >= 0 {
			match := highlighted[idx : idx+len(term)]
			highlighted = highlighted[:idx] + Yellow + Bold + match + Reset + highlighted[idx+len(term):]
		}
	}
	fmt.Printf("  %s│%s  %s\n", Cyan, Reset, highlighted)
}

func SearchSummary(results int, files int) {
	fmt.Printf("\n  %s%d results across %d notes%s\n", Dim, results, files, Reset)
}

func NoteContent(content string) {
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	lineNum := 0

	for _, line := range lines {
		if line == "---" {
			inFrontmatter = !inFrontmatter
			continue
		}
		if inFrontmatter {
			continue
		}
		lineNum++
		fmt.Printf("  %s%3d%s │ %s\n", Dim, lineNum, Reset, line)
	}
}

func ListEntry(title string, tags []string, date string) {
	fmt.Printf("  %s%-30s%s", White, title, Reset)
	if len(tags) > 0 {
		tagStrs := []string{}
		for _, t := range tags {
			t = strings.TrimSpace(t)
			if t != "" {
				tagStrs = append(tagStrs, "#"+t)
			}
		}
		fmt.Printf("  %s%s%s", Cyan, strings.Join(tagStrs, " "), Reset)
	}
	fmt.Printf("  %s%s%s\n", Dim, date, Reset)
}

func TagEntry(tag string, count int) {
	fmt.Printf("  %s#%-20s%s %s%d notes%s\n", Cyan, tag, Reset, Dim, count, Reset)
}

func StatLine(label string, value interface{}) {
	fmt.Printf("  %-20s %s%v%s\n", label, White, value, Reset)
}

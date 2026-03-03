package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/butwhoistrace/note/internal/crypto"
	"github.com/butwhoistrace/note/internal/display"
	"github.com/butwhoistrace/note/internal/index"
	"github.com/butwhoistrace/note/internal/meta"
	"github.com/butwhoistrace/note/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	s, err := store.New()
	if err != nil {
		display.Error(fmt.Sprintf("Failed to initialize: %v", err))
		os.Exit(1)
	}

	idx := index.New(s.BaseDir)
	if err := idx.Load(); err != nil {
		display.Error(fmt.Sprintf("Warning: index corrupted, run 'note reindex': %v", err))
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "new":
		cmdNew(s, idx, args)
	case "add":
		cmdAdd(s, idx, args)
	case "quick":
		cmdQuick(s, idx, args)
	case "show":
		cmdShow(s, args)
	case "edit":
		cmdEdit(s, idx, args)
	case "rm":
		cmdRm(s, idx, args)
	case "delete":
		cmdDelete(s, idx, args)
	case "restore":
		cmdRestore(s, idx, args)
	case "rename":
		cmdRename(s, idx, args)
	case "search":
		cmdSearch(s, idx, args)
	case "list":
		cmdList(s, args)
	case "tags":
		cmdTags(s)
	case "tag":
		cmdTagAdd(s, idx, args)
	case "untag":
		cmdUntag(s, idx, args)
	case "timeline":
		cmdTimeline(s, args)
	case "stats":
		cmdStats(s)
	case "reindex":
		cmdReindex(s, idx)
	case "trash":
		cmdTrash(s, args)
	case "doctor":
		cmdDoctor(s, idx)
	case "init":
		cmdInit(s)
	case "sync":
		cmdSync(s)
	case "pull":
		cmdPull(s)
	case "export":
		cmdExport(s, args)
	case "import":
		cmdImport(s, idx, args)
	case "encrypt":
		cmdEncrypt(s, idx, args)
	case "decrypt":
		cmdDecrypt(s, idx, args)
	case "lock":
		cmdLock(s, idx)
	case "unlock":
		cmdUnlock(s, idx)
	case "tree":
		cmdTree(s)
	case "hook":
		cmdHook(s, args)
	case "help":
		printUsage()
	default:
		display.Error(fmt.Sprintf("Unknown command: %s", cmd))
		printUsage()
	}

	runHooks(s, cmd)
}

func getFlag(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func getPositional(args []string) []string {
	var pos []string
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if a == "--tag" || a == "--sort" || a == "--format" || a == "--to" || a == "--context" {
			skip = true
			continue
		}
		if strings.HasPrefix(a, "--") {
			continue
		}
		pos = append(pos, a)
	}
	return pos
}

func readPassword(prompt string) []byte {
	fmt.Printf("  %s", prompt)
	reader := bufio.NewReader(os.Stdin)
	pw, _ := reader.ReadString('\n')
	return []byte(strings.TrimSpace(pw))
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// === CORE COMMANDS ===

func cmdNew(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 1 {
		display.Error("Usage: note new \"Name\" [--tag tag1 tag2]")
		return
	}
	title := pos[0]
	var tags []string
	for i, a := range args {
		if a == "--tag" {
			for j := i + 1; j < len(args); j++ {
				if strings.HasPrefix(args[j], "--") {
					break
				}
				tags = append(tags, args[j])
			}
			break
		}
	}
	if err := s.CreateNote(title, tags); err != nil {
		display.Error(err.Error())
		return
	}
	idx.IndexFile(s.NotePath(title))
	idx.Save()
	display.Success(fmt.Sprintf("Created: %s.md", s.Slugify(title)))
}

func cmdAdd(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 1 {
		display.Error("Usage: note add \"Name\" \"text\"")
		return
	}
	name := pos[0]
	var text string
	if len(pos) >= 2 {
		text = strings.Join(pos[1:], " ")
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			display.Error("Failed to read stdin")
			return
		}
		text = strings.TrimSpace(string(data))
	}
	path, err := s.AddLine(name, text)
	if err != nil {
		display.Error(err.Error())
		return
	}
	idx.IndexFile(path)
	idx.Save()
	lines := strings.Count(text, "\n") + 1
	display.Success(fmt.Sprintf("Added %d line(s) to %s", lines, name))
}

func cmdQuick(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 1 {
		display.Error("Usage: note quick \"text\" [--tag tag]")
		return
	}
	text := strings.Join(pos, " ")
	tagStr := getFlag(args, "--tag")
	quickPath := filepath.Join(s.NotesDir, "quick-notes.md")
	if _, err := os.Stat(quickPath); os.IsNotExist(err) {
		tags := ""
		if tagStr != "" {
			tags = tagStr
		}
		s.CreateNote("Quick Notes", strings.Fields(tags))
	}
	if _, err := s.AddLine("Quick Notes", text); err != nil {
		display.Error(err.Error())
		return
	}
	idx.IndexFile(quickPath)
	idx.Save()
	display.Success("Saved to quick-notes.md")
}

func cmdShow(s *store.Store, args []string) {
	if hasFlag(args, "--last") {
		entries, _ := os.ReadDir(s.NotesDir)
		var latest os.DirEntry
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				if latest == nil {
					latest = e
				} else {
					li, _ := latest.Info()
					ei, _ := e.Info()
					if ei.ModTime().After(li.ModTime()) {
						latest = e
					}
				}
			}
		}
		if latest == nil {
			display.Error("No notes found.")
			return
		}
		name := strings.TrimSuffix(latest.Name(), ".md")
		args = []string{name}
	}
	pos := getPositional(args)
	if len(pos) < 1 {
		display.Error("Usage: note show \"Name\" or note show --last")
		return
	}
	content, err := s.ReadNote(pos[0])
	if err != nil {
		display.Error(err.Error())
		return
	}
	fm := meta.Parse(content)
	fmt.Println()
	display.NoteTitle(fm.Title)
	if len(fm.Tags) > 0 {
		display.NoteTags(fm.Tags)
	}
	display.Info(fm.Created)
	fmt.Println()
	display.NoteContent(content)
	fmt.Println()
}

func cmdEdit(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 3 {
		display.Error("Usage: note edit \"Name\" <line> \"new text\"")
		return
	}
	lineNum, err := strconv.Atoi(pos[1])
	if err != nil {
		display.Error("Line number must be a number")
		return
	}
	newText := strings.Join(pos[2:], " ")
	if err := s.EditLine(pos[0], lineNum, newText); err != nil {
		display.Error(err.Error())
		return
	}
	idx.IndexFile(s.NotePath(pos[0]))
	idx.Save()
	display.Success(fmt.Sprintf("Updated line %d", lineNum))
}

func cmdRm(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 2 {
		display.Error("Usage: note rm \"Name\" <line>")
		return
	}
	lineNum, err := strconv.Atoi(pos[1])
	if err != nil {
		display.Error("Line number must be a number")
		return
	}
	if err := s.RemoveLine(pos[0], lineNum); err != nil {
		display.Error(err.Error())
		return
	}
	idx.IndexFile(s.NotePath(pos[0]))
	idx.Save()
	display.Success(fmt.Sprintf("Removed line %d", lineNum))
}

func cmdDelete(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 1 {
		display.Error("Usage: note delete \"Name\"")
		return
	}
	path := s.NotePath(pos[0])
	if err := s.DeleteNote(pos[0]); err != nil {
		display.Error(err.Error())
		return
	}
	idx.RemoveFile(path)
	idx.Save()
	display.Success(fmt.Sprintf("Moved to trash: %s", pos[0]))
}

func cmdRestore(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 1 {
		display.Error("Usage: note restore \"Name\"")
		return
	}
	if err := s.RestoreNote(pos[0]); err != nil {
		display.Error(err.Error())
		return
	}
	idx.IndexFile(s.NotePath(pos[0]))
	idx.Save()
	display.Success(fmt.Sprintf("Restored: %s", pos[0]))
}

func cmdRename(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 2 {
		display.Error("Usage: note rename \"Old\" \"New\"")
		return
	}
	oldPath := s.NotePath(pos[0])
	if err := s.RenameNote(pos[0], pos[1]); err != nil {
		display.Error(err.Error())
		return
	}
	idx.RemoveFile(oldPath)
	idx.IndexFile(s.NotePath(pos[1]))
	idx.Save()
	display.Success(fmt.Sprintf("Renamed: %s -> %s", pos[0], pos[1]))
}

// === SEARCH ===

func cmdSearch(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 1 {
		display.Error("Usage: note search \"query\" [--tag t] [--fuzzy] [--regex] [--context N]")
		return
	}
	query := strings.Join(pos, " ")
	tagFilter := getFlag(args, "--tag")
	ctxStr := getFlag(args, "--context")
	ctxLines := 0
	if ctxStr != "" {
		ctxLines, _ = strconv.Atoi(ctxStr)
	}

	var results []index.SearchResult

	if hasFlag(args, "--regex") {
		var err error
		results, err = idx.RegexSearch(query, s.NotesDir)
		if err != nil {
			display.Error(fmt.Sprintf("Invalid regex: %v", err))
			return
		}
	} else if hasFlag(args, "--fuzzy") {
		results = idx.FuzzySearch(query, s.NotesDir, 2)
	} else {
		results = idx.Search(query, s.NotesDir)
	}

	if tagFilter != "" {
		var filtered []index.SearchResult
		for _, r := range results {
			path := filepath.Join(s.NotesDir, r.File)
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			fm := meta.Parse(string(content))
			for _, t := range fm.Tags {
				if strings.EqualFold(strings.TrimSpace(t), tagFilter) {
					filtered = append(filtered, r)
					break
				}
			}
		}
		results = filtered
	}

	if len(results) == 0 {
		display.Info("No results found.")
		return
	}

	files := make(map[string]bool)
	for _, r := range results {
		files[r.File] = true
		display.SearchResult(r.File, r.Line, r.Content, query)

		if ctxLines > 0 {
			ctx := index.GetContext(s.NotesDir, r.File, r.Line, ctxLines)
			for _, cl := range ctx {
				if cl != r.Content {
					fmt.Printf("  %s|%s  %s%s%s\n", display.Cyan, display.Reset, display.Dim, cl, display.Reset)
				}
			}
		}
	}
	display.SearchSummary(len(results), len(files))
}

// === ORGANIZE ===

func cmdList(s *store.Store, args []string) {
	tagFilter := getFlag(args, "--tag")
	sortBy := getFlag(args, "--sort")
	if sortBy == "" {
		sortBy = "date"
	}
	notes, err := s.ListNotes(tagFilter, sortBy)
	if err != nil {
		display.Error(err.Error())
		return
	}
	if len(notes) == 0 {
		display.Info("No notes found.")
		return
	}
	fmt.Println()
	for _, n := range notes {
		display.ListEntry(n.Title, n.Tags, n.Created.Format("2006-01-02"))
	}
	fmt.Println()
	display.Info(fmt.Sprintf("%d notes", len(notes)))
}

func cmdTags(s *store.Store) {
	tags, err := s.ListTags()
	if err != nil {
		display.Error(err.Error())
		return
	}
	if len(tags) == 0 {
		display.Info("No tags found.")
		return
	}
	type tagCount struct {
		tag   string
		count int
	}
	var sorted []tagCount
	for t, c := range tags {
		sorted = append(sorted, tagCount{t, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})
	fmt.Println()
	for _, tc := range sorted {
		display.TagEntry(tc.tag, tc.count)
	}
	fmt.Println()
}

func cmdTagAdd(s *store.Store, idx *index.Index, args []string) {
	if len(args) < 2 {
		display.Error("Usage: note tag \"Name\" tag1 tag2")
		return
	}
	name := args[0]
	tags := args[1:]
	if err := s.AddTags(name, tags); err != nil {
		display.Error(err.Error())
		return
	}
	idx.IndexFile(s.NotePath(name))
	idx.Save()
	display.Success(fmt.Sprintf("Added tags to %s: %s", name, strings.Join(tags, ", ")))
}

func cmdUntag(s *store.Store, idx *index.Index, args []string) {
	if len(args) < 2 {
		display.Error("Usage: note untag \"Name\" tag")
		return
	}
	if err := s.RemoveTag(args[0], args[1]); err != nil {
		display.Error(err.Error())
		return
	}
	idx.IndexFile(s.NotePath(args[0]))
	idx.Save()
	display.Success(fmt.Sprintf("Removed tag '%s' from %s", args[1], args[0]))
}

// === OVERVIEW ===

func cmdTimeline(s *store.Store, args []string) {
	notes, err := s.ListNotes("", "date")
	if err != nil {
		display.Error(err.Error())
		return
	}
	fmt.Println()
	currentMonth := ""
	for _, n := range notes {
		month := n.Created.Format("January 2006")
		if month != currentMonth {
			currentMonth = month
			fmt.Printf("\n  %s%s%s%s\n", display.Bold, display.White, month, display.Reset)
		}
		display.ListEntry(n.Title, n.Tags, n.Created.Format("2006-01-02"))
	}
	fmt.Println()
}

func cmdStats(s *store.Store) {
	noteCount, trashCount, tags, err := s.Stats()
	if err != nil {
		display.Error(err.Error())
		return
	}
	paths, _ := s.GetAllNotePaths()
	var totalSize int64
	for _, p := range paths {
		info, err := os.Stat(p)
		if err == nil {
			totalSize += info.Size()
		}
	}
	fmt.Println()
	display.StatLine("Notes:", noteCount)
	display.StatLine("In trash:", trashCount)
	display.StatLine("Tags:", len(tags))
	display.StatLine("Total size:", formatSize(totalSize))
	if len(tags) > 0 {
		fmt.Printf("\n  %sTop tags:%s\n", display.Bold, display.Reset)
		type tc struct {
			t string
			c int
		}
		var sorted []tc
		for t, c := range tags {
			sorted = append(sorted, tc{t, c})
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].c > sorted[j].c
		})
		limit := 5
		if len(sorted) < limit {
			limit = len(sorted)
		}
		for _, s := range sorted[:limit] {
			display.TagEntry(s.t, s.c)
		}
	}
	fmt.Println()
}

// === TREE ===

func cmdTree(s *store.Store) {
	tags, err := s.ListTags()
	if err != nil {
		display.Error(err.Error())
		return
	}

	notes, _ := s.ListNotes("", "name")

	fmt.Println()
	fmt.Printf("  %s%s~/.note/%s\n", display.Bold, display.Cyan, display.Reset)

	// Group by tags
	tagNotes := make(map[string][]string)
	untagged := []string{}
	for _, n := range notes {
		if len(n.Tags) == 0 || (len(n.Tags) == 1 && strings.TrimSpace(n.Tags[0]) == "") {
			untagged = append(untagged, n.Title)
			continue
		}
		for _, t := range n.Tags {
			t = strings.TrimSpace(t)
			if t != "" {
				tagNotes[t] = append(tagNotes[t], n.Title)
			}
		}
	}

	// Sort tag names
	var tagNames []string
	for t := range tags {
		tagNames = append(tagNames, t)
	}
	sort.Strings(tagNames)

	for i, t := range tagNames {
		isLast := i == len(tagNames)-1 && len(untagged) == 0
		connector := "├"
		if isLast {
			connector = "└"
		}
		fmt.Printf("  %s%s── %s#%s%s\n", display.Dim, connector, display.Cyan, t, display.Reset)
		for j, n := range tagNotes[t] {
			subConn := "│  ├"
			if j == len(tagNotes[t])-1 {
				subConn = "│  └"
			}
			if isLast {
				subConn = strings.Replace(subConn, "│", " ", 1)
			}
			fmt.Printf("  %s%s── %s%s%s\n", display.Dim, subConn, display.White, n, display.Reset)
		}
	}

	if len(untagged) > 0 {
		fmt.Printf("  %s└── %suntagged%s\n", display.Dim, display.Yellow, display.Reset)
		for j, n := range untagged {
			subConn := "   ├"
			if j == len(untagged)-1 {
				subConn = "   └"
			}
			fmt.Printf("  %s%s── %s%s%s\n", display.Dim, subConn, display.White, n, display.Reset)
		}
	}
	fmt.Println()
}

// === ENCRYPT / DECRYPT / LOCK / UNLOCK ===

func cmdEncrypt(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 1 {
		display.Error("Usage: note encrypt \"Name\"")
		return
	}
	path := s.NotePath(pos[0])
	if _, err := os.Stat(path); err != nil {
		display.Error(fmt.Sprintf("Note not found: %s", pos[0]))
		return
	}
	pw := readPassword("Password: ")
	if len(pw) == 0 {
		display.Error("Password cannot be empty.")
		return
	}
	defer zeroBytes(pw)
	if err := crypto.EncryptFile(path, pw); err != nil {
		display.Error(err.Error())
		return
	}
	idx.RemoveFile(path)
	idx.Save()
	display.Success(fmt.Sprintf("Encrypted: %s", pos[0]))
}

func cmdDecrypt(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 1 {
		display.Error("Usage: note decrypt \"Name\"")
		return
	}
	encPath := s.NotePath(pos[0]) + ".enc"
	if _, err := os.Stat(encPath); err != nil {
		display.Error(fmt.Sprintf("Encrypted note not found: %s", pos[0]))
		return
	}
	pw := readPassword("Password: ")
	defer zeroBytes(pw)
	plaintext, err := crypto.DecryptFile(encPath, pw)
	if err != nil {
		display.Error(err.Error())
		return
	}
	if err := crypto.RestoreFile(encPath, plaintext); err != nil {
		display.Error(err.Error())
		return
	}
	idx.IndexFile(s.NotePath(pos[0]))
	idx.Save()
	display.Success(fmt.Sprintf("Decrypted: %s", pos[0]))
}

func cmdLock(s *store.Store, idx *index.Index) {
	pw := readPassword("Lock password: ")
	if len(pw) == 0 {
		display.Error("Password cannot be empty.")
		return
	}
	defer zeroBytes(pw)
	paths, err := s.GetAllNotePaths()
	if err != nil {
		display.Error(err.Error())
		return
	}
	count := 0
	for _, p := range paths {
		if err := crypto.EncryptFile(p, pw); err == nil {
			count++
		}
	}
	// Also encrypt index
	idxPath := filepath.Join(s.BaseDir, ".index")
	crypto.EncryptFile(idxPath, pw)

	display.Success(fmt.Sprintf("Locked %d notes.", count))
}

func cmdUnlock(s *store.Store, idx *index.Index) {
	pw := readPassword("Unlock password: ")
	defer zeroBytes(pw)

	// Decrypt index first
	idxPath := filepath.Join(s.BaseDir, ".index")
	if _, err := os.Stat(idxPath + ".enc"); err == nil {
		plaintext, err := crypto.DecryptFile(idxPath+".enc", pw)
		if err != nil {
			display.Error("Wrong password.")
			return
		}
		crypto.RestoreFile(idxPath+".enc", plaintext)
	}

	// Decrypt all notes
	entries, err := os.ReadDir(s.NotesDir)
	if err != nil {
		display.Error(err.Error())
		return
	}
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".enc") {
			p := filepath.Join(s.NotesDir, e.Name())
			plaintext, err := crypto.DecryptFile(p, pw)
			if err != nil {
				continue
			}
			crypto.RestoreFile(p, plaintext)
			count++
		}
	}

	idx.Load()
	display.Success(fmt.Sprintf("Unlocked %d notes.", count))
}

// === HOOKS ===

func cmdHook(s *store.Store, args []string) {
	if len(args) < 2 {
		display.Error("Usage: note hook <event> <command>")
		display.Info("Events: new, add, delete, sync, search")
		display.Info("Example: note hook sync \"git push origin main\"")
		return
	}
	event := args[0]
	command := strings.Join(args[1:], " ")

	hookFile := filepath.Join(s.BaseDir, ".hooks")
	f, err := os.OpenFile(hookFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		display.Error(err.Error())
		return
	}
	defer f.Close()

	f.WriteString(event + ":" + command + "\n")
	display.Success(fmt.Sprintf("Hook added: on '%s' run '%s'", event, command))
}

func runHooks(s *store.Store, event string) {
	hookFile := filepath.Join(s.BaseDir, ".hooks")
	data, err := os.ReadFile(hookFile)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) == event {
			hookCmd := strings.TrimSpace(parts[1])
			// Split into args to avoid shell injection
			fields := strings.Fields(hookCmd)
			if len(fields) == 0 {
				continue
			}
			cmd := exec.Command(fields[0], fields[1:]...)
			cmd.Dir = s.BaseDir
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		}
	}
}

// === SYNC ===

func cmdInit(s *store.Store) {
	cmd := exec.Command("git", "init")
	cmd.Dir = s.BaseDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		display.Error(fmt.Sprintf("Git init failed: %s", string(out)))
		return
	}
	display.Success("Git repo initialized in ~/.note/")
}

func cmdSync(s *store.Store) {
	commands := [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "sync"},
		{"git", "push"},
	}
	for _, c := range commands {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = s.BaseDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(out), "nothing to commit") {
				display.Info("Nothing to sync.")
				return
			}
			display.Error(fmt.Sprintf("%s: %s", strings.Join(c, " "), string(out)))
			return
		}
	}
	display.Success("Synced.")
}

func cmdPull(s *store.Store) {
	cmd := exec.Command("git", "pull")
	cmd.Dir = s.BaseDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		display.Error(fmt.Sprintf("Pull failed: %s", string(out)))
		return
	}
	display.Success("Pulled.")
}

// === EXPORT / IMPORT ===

func cmdExport(s *store.Store, args []string) {
	format := getFlag(args, "--format")
	if format == "" {
		format = "md"
	}
	pos := getPositional(args)

	if hasFlag(args, "--all") || hasFlag(args, "--tag") {
		tagFilter := getFlag(args, "--tag")
		notes, err := s.ListNotes(tagFilter, "date")
		if err != nil {
			display.Error(err.Error())
			return
		}
		if format == "zip" {
			exportDir := "note-export"
			os.MkdirAll(exportDir, 0755)
			for _, n := range notes {
				content, _ := s.ReadNote(n.Title)
				os.WriteFile(filepath.Join(exportDir, s.Slugify(n.Title)+".md"), []byte(content), 0644)
			}
			cmd := exec.Command("zip", "-r", "note-export.zip", exportDir)
			cmd.Run()
			os.RemoveAll(exportDir)
			display.Success(fmt.Sprintf("Exported %d notes to note-export.zip", len(notes)))
		} else {
			for _, n := range notes {
				content, _ := s.ReadNote(n.Title)
				fname := s.Slugify(n.Title) + "." + format
				os.WriteFile(fname, []byte(content), 0644)
			}
			display.Success(fmt.Sprintf("Exported %d notes as .%s", len(notes), format))
		}
		return
	}

	if len(pos) > 0 {
		content, err := s.ReadNote(pos[0])
		if err != nil {
			display.Error(err.Error())
			return
		}
		fname := s.Slugify(pos[0]) + "." + format
		os.WriteFile(fname, []byte(content), 0644)
		display.Success(fmt.Sprintf("Exported: %s", fname))
	} else {
		display.Error("Usage: note export \"Name\" --format md")
	}
}

func cmdImport(s *store.Store, idx *index.Index, args []string) {
	pos := getPositional(args)
	if len(pos) < 1 {
		display.Error("Usage: note import file.md [--to \"Name\"]")
		return
	}
	data, err := os.ReadFile(pos[0])
	if err != nil {
		display.Error(fmt.Sprintf("Cannot read: %s", pos[0]))
		return
	}
	target := getFlag(args, "--to")
	if target != "" {
		path, err := s.AddLine(target, string(data))
		if err != nil {
			display.Error(err.Error())
			return
		}
		idx.IndexFile(path)
		idx.Save()
		display.Success(fmt.Sprintf("Imported to %s", target))
	} else {
		name := strings.TrimSuffix(filepath.Base(pos[0]), filepath.Ext(pos[0]))
		dest := filepath.Join(s.NotesDir, s.Slugify(name)+".md")
		if err := os.WriteFile(dest, data, 0644); err != nil {
			display.Error(err.Error())
			return
		}
		idx.IndexFile(dest)
		idx.Save()
		display.Success(fmt.Sprintf("Imported: %s", name))
	}
}

// === MAINTENANCE ===

func cmdReindex(s *store.Store, idx *index.Index) {
	if err := idx.Rebuild(s.NotesDir); err != nil {
		display.Error(err.Error())
		return
	}
	display.Success("Index rebuilt.")
}

func cmdTrash(s *store.Store, args []string) {
	if hasFlag(args, "--clear") {
		if err := s.ClearTrash(); err != nil {
			display.Error(err.Error())
			return
		}
		display.Success("Trash cleared.")
		return
	}
	items, err := s.ListTrash()
	if err != nil {
		display.Error(err.Error())
		return
	}
	if len(items) == 0 {
		display.Info("Trash is empty.")
		return
	}
	fmt.Println()
	for _, name := range items {
		fmt.Printf("  %s%s%s\n", display.Dim, name, display.Reset)
	}
	fmt.Println()
	display.Info(fmt.Sprintf("%d items in trash", len(items)))
}

func cmdDoctor(s *store.Store, idx *index.Index) {
	fmt.Println()
	issues := 0
	paths, err := s.GetAllNotePaths()
	if err != nil {
		display.Error("Cannot read notes directory")
		return
	}
	display.StatLine("Notes found:", len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			display.Error(fmt.Sprintf("Cannot read: %s", filepath.Base(p)))
			issues++
			continue
		}
		content := string(data)
		if !strings.HasPrefix(content, "---") {
			display.Error(fmt.Sprintf("Missing frontmatter: %s", filepath.Base(p)))
			issues++
		}
	}
	idxPath := filepath.Join(s.BaseDir, ".index")
	if _, err := os.Stat(idxPath); os.IsNotExist(err) {
		display.Error("Index file missing. Run: note reindex")
		issues++
	} else {
		info, _ := os.Stat(idxPath)
		display.StatLine("Index size:", formatSize(info.Size()))
	}
	if issues == 0 {
		display.Success("Everything looks good.")
	} else {
		display.Error(fmt.Sprintf("%d issue(s) found.", issues))
	}
	fmt.Println()
}

// === HELPERS ===

func formatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
}

func printUsage() {
	display.Banner()
	fmt.Println("  A minimal knowledge base for your terminal.")
	fmt.Println()
	fmt.Printf("  %sCore%s\n", display.Bold, display.Reset)
	fmt.Println("    note new \"Name\" [--tag t1 t2]     Create a note")
	fmt.Println("    note add \"Name\" \"text\"            Append a line")
	fmt.Println("    note quick \"text\" [--tag t]        Quick one-liner")
	fmt.Println("    note show \"Name\"                   Display a note")
	fmt.Println("    note show --last                   Show last modified")
	fmt.Println("    note edit \"Name\" <line> \"text\"     Replace a line")
	fmt.Println("    note rm \"Name\" <line>              Remove a line")
	fmt.Println("    note delete \"Name\"                 Move to trash")
	fmt.Println("    note restore \"Name\"                Restore from trash")
	fmt.Println("    note rename \"Old\" \"New\"            Rename a note")
	fmt.Println()
	fmt.Printf("  %sSearch%s\n", display.Bold, display.Reset)
	fmt.Println("    note search \"query\"                Full-text search")
	fmt.Println("    note search \"q\" --tag t            Filter by tag")
	fmt.Println("    note search \"q\" --fuzzy            Fuzzy search (typo tolerant)")
	fmt.Println("    note search \"pattern\" --regex      Regex search")
	fmt.Println("    note search \"q\" --context 3        Show surrounding lines")
	fmt.Println()
	fmt.Printf("  %sOrganize%s\n", display.Bold, display.Reset)
	fmt.Println("    note list [--tag t] [--sort name]  List notes")
	fmt.Println("    note tags                          Show all tags")
	fmt.Println("    note tag \"Name\" t1 t2              Add tags")
	fmt.Println("    note untag \"Name\" t1               Remove a tag")
	fmt.Println("    note tree                          Tree view by tags")
	fmt.Println()
	fmt.Printf("  %sOverview%s\n", display.Bold, display.Reset)
	fmt.Println("    note timeline [--week|--month]     Chronological view")
	fmt.Println("    note stats                         Statistics")
	fmt.Println()
	fmt.Printf("  %sSecurity%s\n", display.Bold, display.Reset)
	fmt.Println("    note encrypt \"Name\"                Encrypt a note (AES-256)")
	fmt.Println("    note decrypt \"Name\"                Decrypt a note")
	fmt.Println("    note lock                          Encrypt all notes")
	fmt.Println("    note unlock                        Decrypt all notes")
	fmt.Println()
	fmt.Printf("  %sSync%s\n", display.Bold, display.Reset)
	fmt.Println("    note init                          Init git repo")
	fmt.Println("    note sync                          Git push")
	fmt.Println("    note pull                          Git pull")
	fmt.Println()
	fmt.Printf("  %sHooks%s\n", display.Bold, display.Reset)
	fmt.Println("    note hook <event> <command>         Add a hook")
	fmt.Println("    Events: new, add, delete, sync, search")
	fmt.Println()
	fmt.Printf("  %sExport/Import%s\n", display.Bold, display.Reset)
	fmt.Println("    note export \"Name\" --format md      Export single note")
	fmt.Println("    note export --all --format zip      Export all")
	fmt.Println("    note import file.md [--to \"Name\"]   Import file")
	fmt.Println()
	fmt.Printf("  %sMaintenance%s\n", display.Bold, display.Reset)
	fmt.Println("    note reindex                       Rebuild search index")
	fmt.Println("    note trash [--clear]               View/clear trash")
	fmt.Println("    note doctor                        Health check")
	fmt.Println()
}

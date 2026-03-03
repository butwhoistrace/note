package index

import (
	"bufio"
	"encoding/gob"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/butwhoistrace/note/internal/meta"
)

type Entry struct {
	File string
	Line int
}

type FileMeta struct {
	Title   string
	Tags    []string
	Created string
}

type indexData struct {
	Words map[string][]Entry
	Meta  map[string]FileMeta // filename -> metadata
}

type Index struct {
	path string
	data indexData
	mu   sync.RWMutex
}

func New(baseDir string) *Index {
	return &Index{
		path: filepath.Join(baseDir, ".index"),
		data: indexData{
			Words: make(map[string][]Entry),
			Meta:  make(map[string]FileMeta),
		},
	}
}

func (idx *Index) GetMeta(filename string) (FileMeta, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	m, ok := idx.data.Meta[filename]
	return m, ok
}

func (idx *Index) AllMeta() map[string]FileMeta {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make(map[string]FileMeta, len(idx.data.Meta))
	for k, v := range idx.data.Meta {
		out[k] = v
	}
	return out
}

func (idx *Index) Load() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	f, err := os.Open(idx.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	var d indexData
	if err := gob.NewDecoder(f).Decode(&d); err != nil {
		return err
	}
	if d.Words == nil {
		d.Words = make(map[string][]Entry)
	}
	if d.Meta == nil {
		d.Meta = make(map[string]FileMeta)
	}
	idx.data = d
	return nil
}

func (idx *Index) Save() error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	tmp := idx.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(idx.data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, idx.path)
}

func (idx *Index) IndexFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	fname := filepath.Base(path)
	content := string(data)

	// Parse metadata from frontmatter
	fm := meta.Parse(content)

	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Remove old entries for this file
	for word, entries := range idx.data.Words {
		var filtered []Entry
		for _, e := range entries {
			if e.File != fname {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) > 0 {
			idx.data.Words[word] = filtered
		} else {
			delete(idx.data.Words, word)
		}
	}

	// Store metadata
	idx.data.Meta[fname] = FileMeta{
		Title:   fm.Title,
		Tags:    fm.Tags,
		Created: fm.Created,
	}

	// Index words line by line
	lineNum := 0
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		words := tokenize(line)
		seen := make(map[string]bool)
		for _, w := range words {
			if seen[w] || len(w) < 2 {
				continue
			}
			seen[w] = true
			idx.data.Words[w] = append(idx.data.Words[w], Entry{File: fname, Line: lineNum})
		}
	}
	return scanner.Err()
}

func (idx *Index) RemoveFile(path string) {
	fname := filepath.Base(path)
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for word, entries := range idx.data.Words {
		var filtered []Entry
		for _, e := range entries {
			if e.File != fname {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) > 0 {
			idx.data.Words[word] = filtered
		} else {
			delete(idx.data.Words, word)
		}
	}
	delete(idx.data.Meta, fname)
}

type SearchResult struct {
	File    string
	Line    int
	Content string
}

func (idx *Index) Search(query string, notesDir string) []SearchResult {
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}

	idx.mu.RLock()

	// For each term collect the set of files that contain it
	termFiles := make([]map[string][]int, len(terms))
	for i, term := range terms {
		termFiles[i] = make(map[string][]int)
		entries, ok := idx.data.Words[term]
		if !ok {
			// Try prefix match
			for word, wordEntries := range idx.data.Words {
				if strings.HasPrefix(word, term) {
					entries = append(entries, wordEntries...)
				}
			}
		}
		for _, e := range entries {
			termFiles[i][e.File] = append(termFiles[i][e.File], e.Line)
		}
	}
	idx.mu.RUnlock()

	// AND: only files that appear in every term's file set
	if len(termFiles) == 0 {
		return nil
	}
	candidates := termFiles[0]
	for _, tf := range termFiles[1:] {
		for fname := range candidates {
			if _, ok := tf[fname]; !ok {
				delete(candidates, fname)
			}
		}
	}

	var results []SearchResult
	for fname := range candidates {
		path := filepath.Join(notesDir, fname)
		scanLines(path, func(lineNum int, line string) {
			lineLower := strings.ToLower(line)
			for _, t := range terms {
				if !strings.Contains(lineLower, t) {
					return
				}
			}
			results = append(results, SearchResult{
				File:    fname,
				Line:    lineNum,
				Content: line,
			})
		})
	}
	return results
}

// FuzzySearch finds words with Levenshtein distance <= maxDist
func (idx *Index) FuzzySearch(query string, notesDir string, maxDist int) []SearchResult {
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}

	idx.mu.RLock()
	fileCounts := make(map[string]map[int]bool)
	for _, term := range terms {
		if entries, ok := idx.data.Words[term]; ok {
			for _, e := range entries {
				if fileCounts[e.File] == nil {
					fileCounts[e.File] = make(map[int]bool)
				}
				fileCounts[e.File][e.Line] = true
			}
			continue
		}
		for word, entries := range idx.data.Words {
			if levenshtein(term, word) <= maxDist {
				for _, e := range entries {
					if fileCounts[e.File] == nil {
						fileCounts[e.File] = make(map[int]bool)
					}
					fileCounts[e.File][e.Line] = true
				}
			}
		}
	}
	idx.mu.RUnlock()

	var results []SearchResult
	for fname, lines := range fileCounts {
		path := filepath.Join(notesDir, fname)
		scanLines(path, func(lineNum int, line string) {
			if lines[lineNum] {
				results = append(results, SearchResult{
					File:    fname,
					Line:    lineNum,
					Content: line,
				})
			}
		})
	}
	return results
}

// RegexSearch searches all notes with a regex pattern
func (idx *Index) RegexSearch(pattern string, notesDir string) ([]SearchResult, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(notesDir)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(notesDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				results = append(results, SearchResult{
					File:    e.Name(),
					Line:    i + 1,
					Content: line,
				})
			}
		}
	}
	return results, nil
}

// GetContext returns surrounding lines for a search result
func GetContext(notesDir string, file string, line int, ctxLines int) []string {
	path := filepath.Join(notesDir, file)
	start := line - ctxLines
	end := line + ctxLines
	if start < 1 {
		start = 1
	}
	var ctx []string
	scanLines(path, func(lineNum int, text string) {
		if lineNum >= start && lineNum <= end {
			ctx = append(ctx, text)
		}
	})
	return ctx
}

// scanLines calls fn(lineNum, text) for each line in path using a fixed-size buffer.
// lineNum is 1-based. No heap allocation per line beyond the callback.
func scanLines(path string, fn func(lineNum int, line string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		fn(lineNum, scanner.Text())
	}
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			min := del
			if ins < min {
				min = ins
			}
			if sub < min {
				min = sub
			}
			curr[j] = min
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func (idx *Index) Rebuild(notesDir string) error {
	idx.mu.Lock()
	idx.data = indexData{
		Words: make(map[string][]Entry),
		Meta:  make(map[string]FileMeta),
	}
	idx.mu.Unlock()

	entries, err := os.ReadDir(notesDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			if err := idx.IndexFile(filepath.Join(notesDir, e.Name())); err != nil {
				return err
			}
		}
	}
	return idx.Save()
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	var words []string
	word := strings.Builder{}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == ':' || r == '/' || r == '-' || r == '_' || r == '@' {
			word.WriteRune(r)
		} else {
			if word.Len() > 0 {
				words = append(words, word.String())
				word.Reset()
			}
		}
	}
	if word.Len() > 0 {
		words = append(words, word.String())
	}
	return words
}

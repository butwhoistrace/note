package index

import (
	"bufio"
	"encoding/json"
	"regexp"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Entry struct {
	File string `json:"f"`
	Line int    `json:"l"`
}

type Index struct {
	path    string
	data    map[string][]Entry
	mu      sync.RWMutex
}

func New(baseDir string) *Index {
	return &Index{
		path: filepath.Join(baseDir, ".index"),
		data: make(map[string][]Entry),
	}
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

	return json.NewDecoder(f).Decode(&idx.data)
}

func (idx *Index) Save() error {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	f, err := os.Create(idx.path)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(idx.data)
}

func (idx *Index) IndexFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fname := filepath.Base(path)

	idx.mu.Lock()
	// Remove old entries for this file
	for word, entries := range idx.data {
		var filtered []Entry
		for _, e := range entries {
			if e.File != fname {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) > 0 {
			idx.data[word] = filtered
		} else {
			delete(idx.data, word)
		}
	}
	idx.mu.Unlock()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	idx.mu.Lock()
	defer idx.mu.Unlock()

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
			idx.data[w] = append(idx.data[w], Entry{File: fname, Line: lineNum})
		}
	}
	return scanner.Err()
}

func (idx *Index) RemoveFile(path string) {
	fname := filepath.Base(path)
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for word, entries := range idx.data {
		var filtered []Entry
		for _, e := range entries {
			if e.File != fname {
				filtered = append(filtered, e)
			}
		}
		if len(filtered) > 0 {
			idx.data[word] = filtered
		} else {
			delete(idx.data, word)
		}
	}
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

	// Find files that contain ALL terms
	fileCounts := make(map[string]map[int]bool)
	for _, term := range terms {
		entries, ok := idx.data[term]
		if !ok {
			// Try prefix match
			for word, wordEntries := range idx.data {
				if strings.HasPrefix(word, term) {
					entries = append(entries, wordEntries...)
				}
			}
		}
		for _, e := range entries {
			if fileCounts[e.File] == nil {
				fileCounts[e.File] = make(map[int]bool)
			}
			fileCounts[e.File][e.Line] = true
		}
	}
	idx.mu.RUnlock()

	var results []SearchResult
	for fname, lines := range fileCounts {
		path := filepath.Join(notesDir, fname)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fileLines := strings.Split(string(content), "\n")
		for lineNum := range lines {
			if lineNum < 1 || lineNum > len(fileLines) {
				continue
			}
			line := fileLines[lineNum-1]
			// Verify all terms exist in line or nearby lines
			lineLower := strings.ToLower(line)
			match := false
			for _, t := range terms {
				if strings.Contains(lineLower, t) {
					match = true
					break
				}
			}
			if match {
				results = append(results, SearchResult{
					File:    fname,
					Line:    lineNum,
					Content: line,
				})
			}
		}
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
		if entries, ok := idx.data[term]; ok {
			for _, e := range entries {
				if fileCounts[e.File] == nil {
					fileCounts[e.File] = make(map[int]bool)
				}
				fileCounts[e.File][e.Line] = true
			}
			continue
		}
		for word, entries := range idx.data {
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
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fileLines := strings.Split(string(content), "\n")
		for lineNum := range lines {
			if lineNum < 1 || lineNum > len(fileLines) {
				continue
			}
			results = append(results, SearchResult{
				File:    fname,
				Line:    lineNum,
				Content: fileLines[lineNum-1],
			})
		}
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
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	start := line - ctxLines - 1
	end := line + ctxLines
	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	return lines[start:end]
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
	idx.data = make(map[string][]Entry)
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

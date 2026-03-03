package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/butwhoistrace/note/internal/meta"
)

type NoteMeta struct {
	Title   string
	Tags    []string
	Created time.Time
}

type Store struct {
	BaseDir  string
	NotesDir string
	TrashDir string
}

func New() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(home, ".note")
	s := &Store{
		BaseDir:  base,
		NotesDir: filepath.Join(base, "notes"),
		TrashDir: filepath.Join(base, ".trash"),
	}
	if err := os.MkdirAll(s.NotesDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.TrashDir, 0755); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Slugify(name string) string {
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	safe := strings.Builder{}
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			safe.WriteRune(r)
		}
	}
	return safe.String()
}

func (s *Store) NotePath(name string) string {
	return filepath.Join(s.NotesDir, s.Slugify(name)+".md")
}

func (s *Store) TrashPath(name string) string {
	return filepath.Join(s.TrashDir, s.Slugify(name)+".md")
}

func (s *Store) CreateNote(title string, tags []string) error {
	path := s.NotePath(title)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("note already exists: %s", title)
	}

	frontmatter := fmt.Sprintf("---\ntitle: %s\ntags: %s\ncreated: %s\n---\n",
		title,
		strings.Join(tags, ", "),
		time.Now().Format("2006-01-02"),
	)

	return os.WriteFile(path, []byte(frontmatter), 0644)
}

func (s *Store) AddLine(name string, text string) (string, error) {
	path := s.findNote(name)
	if path == "" {
		return "", fmt.Errorf("note not found: %s", name)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = f.WriteString(text + "\n")
	return path, err
}

func (s *Store) ReadNote(name string) (string, error) {
	path := s.findNote(name)
	if path == "" {
		return "", fmt.Errorf("note not found: %s", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Store) DeleteNote(name string) error {
	path := s.findNote(name)
	if path == "" {
		return fmt.Errorf("note not found: %s", name)
	}
	dest := filepath.Join(s.TrashDir, filepath.Base(path))
	return os.Rename(path, dest)
}

func (s *Store) RestoreNote(name string) error {
	slug := s.Slugify(name) + ".md"
	src := filepath.Join(s.TrashDir, slug)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("note not found in trash: %s", name)
	}
	dest := filepath.Join(s.NotesDir, slug)
	return os.Rename(src, dest)
}

func (s *Store) RenameNote(oldName, newName string) error {
	oldPath := s.findNote(oldName)
	if oldPath == "" {
		return fmt.Errorf("note not found: %s", oldName)
	}
	newPath := s.NotePath(newName)

	data, err := os.ReadFile(oldPath)
	if err != nil {
		return err
	}
	content := string(data)
	content = strings.Replace(content, "title: "+oldName, "title: "+newName, 1)

	if err := os.WriteFile(newPath, []byte(content), 0644); err != nil {
		return err
	}
	return os.Remove(oldPath)
}

func (s *Store) EditLine(name string, lineNum int, newText string) error {
	path := s.findNote(name)
	if path == "" {
		return fmt.Errorf("note not found: %s", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	if lineNum < 1 || lineNum > len(lines) {
		return fmt.Errorf("line %d out of range (1-%d)", lineNum, len(lines))
	}
	lines[lineNum-1] = newText
	return writeAtomic(path, []byte(strings.Join(lines, "\n")))
}

func (s *Store) RemoveLine(name string, lineNum int) error {
	path := s.findNote(name)
	if path == "" {
		return fmt.Errorf("note not found: %s", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	if lineNum < 1 || lineNum > len(lines) {
		return fmt.Errorf("line %d out of range (1-%d)", lineNum, len(lines))
	}
	lines = append(lines[:lineNum-1], lines[lineNum:]...)
	return writeAtomic(path, []byte(strings.Join(lines, "\n")))
}

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) ListNotes(tagFilter string, sortBy string) ([]NoteMeta, error) {
	entries, err := os.ReadDir(s.NotesDir)
	if err != nil {
		return nil, err
	}
	var notes []NoteMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(s.NotesDir, e.Name())
		meta := s.parseMeta(path)
		if tagFilter != "" {
			found := false
			for _, t := range meta.Tags {
				if strings.EqualFold(strings.TrimSpace(t), tagFilter) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		notes = append(notes, meta)
	}

	if sortBy == "name" {
		sortByName(notes)
	} else {
		sortByDate(notes)
	}
	return notes, nil
}

func (s *Store) ListTags() (map[string]int, error) {
	entries, err := os.ReadDir(s.NotesDir)
	if err != nil {
		return nil, err
	}
	tags := make(map[string]int)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(s.NotesDir, e.Name())
		meta := s.parseMeta(path)
		for _, t := range meta.Tags {
			t = strings.TrimSpace(t)
			if t != "" {
				tags[t]++
			}
		}
	}
	return tags, nil
}

func (s *Store) AddTags(name string, newTags []string) error {
	path := s.findNote(name)
	if path == "" {
		return fmt.Errorf("note not found: %s", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	meta := s.parseMeta(path)
	allTags := meta.Tags
	for _, nt := range newTags {
		found := false
		for _, et := range allTags {
			if strings.EqualFold(strings.TrimSpace(et), nt) {
				found = true
				break
			}
		}
		if !found {
			allTags = append(allTags, nt)
		}
	}
	oldLine := "tags: " + strings.Join(meta.Tags, ", ")
	newLine := "tags: " + strings.Join(allTags, ", ")
	content = strings.Replace(content, oldLine, newLine, 1)
	return os.WriteFile(path, []byte(content), 0644)
}

func (s *Store) RemoveTag(name string, tag string) error {
	path := s.findNote(name)
	if path == "" {
		return fmt.Errorf("note not found: %s", name)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	meta := s.parseMeta(path)
	var filtered []string
	for _, t := range meta.Tags {
		if !strings.EqualFold(strings.TrimSpace(t), tag) {
			filtered = append(filtered, t)
		}
	}
	oldLine := "tags: " + strings.Join(meta.Tags, ", ")
	newLine := "tags: " + strings.Join(filtered, ", ")
	content = strings.Replace(content, oldLine, newLine, 1)
	return os.WriteFile(path, []byte(content), 0644)
}

func (s *Store) ListTrash() ([]string, error) {
	entries, err := os.ReadDir(s.TrashDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	return names, nil
}

func (s *Store) ClearTrash() error {
	entries, err := os.ReadDir(s.TrashDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		os.Remove(filepath.Join(s.TrashDir, e.Name()))
	}
	return nil
}

func (s *Store) GetAllNotePaths() ([]string, error) {
	entries, err := os.ReadDir(s.NotesDir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			paths = append(paths, filepath.Join(s.NotesDir, e.Name()))
		}
	}
	return paths, nil
}

func (s *Store) Stats() (int, int, map[string]int, error) {
	entries, err := os.ReadDir(s.NotesDir)
	if err != nil {
		return 0, 0, nil, err
	}
	tags := make(map[string]int)
	noteCount := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		noteCount++
		meta := s.parseMeta(filepath.Join(s.NotesDir, e.Name()))
		for _, t := range meta.Tags {
			t = strings.TrimSpace(t)
			if t != "" {
				tags[t]++
			}
		}
	}
	trash, err := s.ListTrash()
	if err != nil {
		return 0, 0, nil, err
	}
	return noteCount, len(trash), tags, nil
}

// Internal helpers

func (s *Store) findNote(name string) string {
	// Try exact slug match first
	path := s.NotePath(name)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	// Try exact title match, then prefix match
	entries, err := os.ReadDir(s.NotesDir)
	if err != nil {
		return ""
	}
	nameLower := strings.ToLower(name)
	var matches []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		p := filepath.Join(s.NotesDir, e.Name())
		m := s.parseMeta(p)
		if strings.EqualFold(m.Title, name) {
			return p // exact match wins immediately
		}
		if strings.HasPrefix(strings.ToLower(m.Title), nameLower) {
			matches = append(matches, p)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return ""
}

func (s *Store) parseMeta(path string) NoteMeta {
	data, err := os.ReadFile(path)
	if err != nil {
		return NoteMeta{}
	}
	fm := meta.Parse(string(data))
	created, _ := time.Parse("2006-01-02", fm.Created)
	return NoteMeta{
		Title:   fm.Title,
		Tags:    fm.Tags,
		Created: created,
	}
}

func sortByDate(notes []NoteMeta) {
	sort.Slice(notes, func(i, j int) bool {
		return notes[i].Created.After(notes[j].Created)
	})
}

func sortByName(notes []NoteMeta) {
	sort.Slice(notes, func(i, j int) bool {
		return strings.ToLower(notes[i].Title) < strings.ToLower(notes[j].Title)
	})
}

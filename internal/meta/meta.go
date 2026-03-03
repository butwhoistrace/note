package meta

import (
	"strings"
	"time"
)

type Frontmatter struct {
	Title   string
	Tags    []string
	Created string
}

func Parse(content string) Frontmatter {
	fm := Frontmatter{}
	if !strings.HasPrefix(content, "---") {
		return fm
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return fm
	}
	header := content[3 : end+3]
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "title:") {
			fm.Title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
		} else if strings.HasPrefix(line, "tags:") {
			tagStr := strings.TrimSpace(strings.TrimPrefix(line, "tags:"))
			if tagStr != "" {
				parts := strings.Split(tagStr, ",")
				for _, t := range parts {
					t = strings.TrimSpace(t)
					if t != "" {
						fm.Tags = append(fm.Tags, t)
					}
				}
			}
		} else if strings.HasPrefix(line, "created:") {
			fm.Created = strings.TrimSpace(strings.TrimPrefix(line, "created:"))
		}
	}
	return fm
}

func ParseCreatedTime(content string) time.Time {
	fm := Parse(content)
	t, _ := time.Parse("2006-01-02", fm.Created)
	return t
}

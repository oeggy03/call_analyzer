package service

import (
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxLessonTags     = 8
	maxLessonTagRunes = 64
)

// DefaultLessonTag is the tag applied to a lesson when the learner leaves the
// start field blank. The clock is the machine's local time at lesson start.
func DefaultLessonTag(at time.Time) string {
	if at.IsZero() {
		at = time.Now()
	}
	return at.In(time.Local).Format("lesson_2006-01-02_15-04")
}

// NormalizeLessonTags turns a comma-separated field into the stored tag list.
// A blank field becomes the standard lesson_<date>_<time> tag.
func NormalizeLessonTags(raw string, at time.Time) string {
	seen := make(map[string]struct{})
	tags := make([]string, 0, maxLessonTags)
	for _, part := range strings.Split(raw, ",") {
		tag := strings.Join(strings.Fields(part), " ")
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > maxLessonTagRunes {
			tag = string([]rune(tag)[:maxLessonTagRunes])
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
		if len(tags) == maxLessonTags {
			break
		}
	}
	if len(tags) == 0 {
		return DefaultLessonTag(at)
	}
	return strings.Join(tags, ",")
}

func splitLessonTags(stored string) []string {
	if strings.TrimSpace(stored) == "" {
		return nil
	}
	parts := strings.Split(stored, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

package main

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// TestWailsMethodsMatchFrontendContract keeps the React client and the Wails
// App methods on one list. frontend/src/lib/api.ts is the source of the names.
func TestWailsMethodsMatchFrontendContract(t *testing.T) {
	source, err := os.ReadFile("frontend/src/lib/api.ts")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(source), "export const BACKEND_METHODS = {")
	if start < 0 {
		t.Fatal("BACKEND_METHODS block not found")
	}
	rest := string(source)[start:]
	end := strings.Index(rest, "} as const")
	if end < 0 {
		t.Fatal("BACKEND_METHODS terminator not found")
	}
	matches := regexp.MustCompile(`:\s*'([A-Za-z0-9]+)'`).FindAllStringSubmatch(rest[:end], -1)
	if len(matches) < 10 {
		t.Fatalf("parsed %d backend methods, want the desktop contract", len(matches))
	}

	appType := reflect.TypeOf(&App{})
	expectedResults := map[string]reflect.Type{
		"GenerateManualVocabulary": reflect.TypeOf(ManualVocabularyDraft{}),
		"RecognizeVocabularyPhoto": reflect.TypeOf(PhotoVocabularyDraft{}),
		"ListLessonTranscripts":    reflect.TypeOf([]LessonTranscriptSummary{}),
		"GetLessonTranscript":      reflect.TypeOf(LessonTranscript{}),
		"ListTags":                 reflect.TypeOf([]string{}),
	}
	seen := map[string]bool{}
	for _, match := range matches {
		name := match[1]
		if seen[name] {
			t.Errorf("duplicate backend method %s", name)
		}
		seen[name] = true
		method, ok := appType.MethodByName(name)
		if !ok {
			t.Errorf("App is missing %s from frontend/src/lib/api.ts", name)
			continue
		}
		if method.Type.NumOut() != 2 || method.Type.Out(1).Name() != "error" {
			t.Errorf("%s must return (result, error) for Wails", name)
			continue
		}
		want := reflect.TypeOf(AppSnapshot{})
		if special, ok := expectedResults[name]; ok {
			want = special
		}
		if got := method.Type.Out(0); got != want {
			t.Errorf("%s result = %s, want %s", name, got, want)
		}
	}
	if !seen["GenerateManualVocabulary"] || !seen["RecognizeVocabularyPhoto"] ||
		!seen["ListLessonTranscripts"] || !seen["GetLessonTranscript"] ||
		!seen["SaveManualVocabulary"] || !seen["DeleteLastLesson"] ||
		!seen["DeleteVocabulary"] {
		t.Fatalf("backend contract is missing a current App method: %#v", seen)
	}
}

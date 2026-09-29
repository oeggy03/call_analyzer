package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/oeggy03/call_analyzer/internal/domain"
	"github.com/oeggy03/call_analyzer/internal/storage"
	"github.com/oeggy03/call_analyzer/internal/vocabulary"
)

type VocabularyService struct {
	store      *storage.Store
	dictionary vocabulary.Dictionary
}

func NewVocabularyService(store *storage.Store, dictionary vocabulary.Dictionary) *VocabularyService {
	if dictionary == nil {
		dictionary = vocabulary.NewEmbeddedDictionary()
	}
	return &VocabularyService{store: store, dictionary: dictionary}
}

func ParseExtractionJSON(payload string) (vocabulary.Extraction, error) {
	var extraction vocabulary.Extraction
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&extraction); err != nil {
		return vocabulary.Extraction{}, fmt.Errorf("service: decode extraction: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return vocabulary.Extraction{}, errors.New("service: extraction has trailing JSON")
	} else if !errors.Is(err, io.EOF) {
		return vocabulary.Extraction{}, fmt.Errorf("service: decode trailing extraction: %w", err)
	}
	return extraction, nil
}

func (s *VocabularyService) ProcessExtraction(
	ctx context.Context,
	lessonID string,
	segmentID string,
	transcriptText string,
	extraction vocabulary.Extraction,
) ([]domain.VocabularyEntry, error) {
	return s.processExtraction(ctx, lessonID, segmentID, transcriptText, extraction, false)
}

func (s *VocabularyService) ProcessManualExtraction(
	ctx context.Context,
	lessonID string,
	segmentID string,
	transcriptText string,
	extraction vocabulary.Extraction,
) ([]domain.VocabularyEntry, error) {
	return s.processExtraction(ctx, lessonID, segmentID, transcriptText, extraction, true)
}

func (s *VocabularyService) processExtraction(
	ctx context.Context,
	lessonID string,
	segmentID string,
	transcriptText string,
	extraction vocabulary.Extraction,
	manual bool,
) ([]domain.VocabularyEntry, error) {
	if s.store == nil {
		return nil, errors.New("service: storage is not configured")
	}
	if strings.TrimSpace(transcriptText) == "" {
		return nil, errors.New("service: transcript text is required for evidence validation")
	}
	candidates, err := vocabulary.Deduplicate(extraction.Candidates)
	if err != nil {
		return nil, err
	}
	entries := make([]domain.VocabularyEntry, 0, len(candidates))
	for _, candidate := range candidates {
		for i := range candidate.Evidence {
			if candidate.Evidence[i].LessonID == "" {
				candidate.Evidence[i].LessonID = lessonID
			}
			if candidate.Evidence[i].SegmentID == "" {
				candidate.Evidence[i].SegmentID = segmentID
			}
		}
		entry, err := vocabulary.NormalizeCandidate(candidate, transcriptText)
		if err != nil {
			return nil, err
		}
		if len(entry.Senses) == 0 {
			senses, err := s.dictionary.Lookup(ctx, entry.Simplified, entry.Traditional)
			if err != nil {
				return nil, err
			}
			entry.Senses = senses
		}
		if manual {
			entry.Manual = true
			entry.Priority = "manual"
			entry.Provenance = domain.ProvenanceUser
		}
		entry, err = s.store.Vocabulary().UpsertCandidate(ctx, entry)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

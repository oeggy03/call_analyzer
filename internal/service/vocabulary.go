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
	"github.com/oeggy03/call_analyzer/internal/transcript"
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
	validCandidates := make([]vocabulary.Candidate, 0, len(extraction.Candidates))
	var candidateErrors []error
	for index, candidate := range extraction.Candidates {
		normalized, err := vocabulary.Deduplicate([]vocabulary.Candidate{candidate})
		if err != nil {
			candidateErrors = append(candidateErrors,
				fmt.Errorf("candidate %d: %w", index+1, err))
			continue
		}
		validCandidates = append(validCandidates, normalized[0])
	}
	candidates, err := vocabulary.Deduplicate(validCandidates)
	if err != nil {
		return nil, err
	}
	segments, err := s.evidenceSegments(ctx, lessonID, segmentID)
	if err != nil {
		return nil, err
	}
	entries := make([]domain.VocabularyEntry, 0, len(candidates))
	for _, candidate := range candidates {
		for i := range candidate.Evidence {
			populateEvidenceWindow(
				&candidate.Evidence[i],
				lessonID,
				segmentID,
				segments,
			)
		}
		entry, err := vocabulary.NormalizeCandidate(candidate, transcriptText)
		if err != nil {
			candidateErrors = append(candidateErrors, err)
			continue
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
	if len(entries) == 0 && len(candidateErrors) > 0 {
		return nil, errors.Join(candidateErrors...)
	}
	return entries, nil
}

func (s *VocabularyService) evidenceSegments(
	ctx context.Context,
	lessonID string,
	segmentID string,
) ([]domain.TranscriptSegment, error) {
	if lessonID != "" {
		segments, err := s.store.Transcripts().List(ctx, lessonID)
		if err != nil {
			return nil, err
		}
		if segmentID == "" {
			return segments, nil
		}
		for _, segment := range segments {
			if segment.ID == segmentID {
				return segments, nil
			}
		}
	}
	if segmentID == "" {
		return nil, nil
	}
	segment, err := s.store.Transcripts().Get(ctx, segmentID)
	if err != nil {
		return nil, err
	}
	return []domain.TranscriptSegment{segment}, nil
}

func populateEvidenceWindow(
	evidence *domain.Evidence,
	lessonID string,
	segmentID string,
	segments []domain.TranscriptSegment,
) {
	if evidence == nil {
		return
	}
	var matched *domain.TranscriptSegment
	if evidence.SegmentID != "" {
		for index := range segments {
			if segments[index].ID == evidence.SegmentID {
				matched = &segments[index]
				break
			}
		}
	}
	if matched == nil && segmentID != "" {
		for index := range segments {
			if segments[index].ID == segmentID &&
				evidenceTextMatches(segments[index].Text, evidence.Text) {
				matched = &segments[index]
				break
			}
		}
	}
	if matched == nil {
		for index := range segments {
			if evidenceTextMatches(segments[index].Text, evidence.Text) {
				matched = &segments[index]
				break
			}
		}
	}
	if evidence.LessonID == "" {
		if matched != nil {
			evidence.LessonID = matched.LessonID
		} else {
			evidence.LessonID = lessonID
		}
	}
	if evidence.SegmentID == "" {
		if matched != nil {
			evidence.SegmentID = matched.ID
		} else {
			evidence.SegmentID = segmentID
		}
	}
	if matched == nil {
		return
	}
	if evidence.StartMS == 0 {
		evidence.StartMS = matched.StartMS
	}
	if evidence.EndMS == 0 {
		evidence.EndMS = matched.EndMS
	}
	if evidence.Confidence == 0 {
		evidence.Confidence = matched.Confidence
	}
}

func evidenceTextMatches(transcriptText, evidenceText string) bool {
	transcriptText = strings.TrimSpace(transcriptText)
	evidenceText = strings.TrimSpace(evidenceText)
	if transcriptText == "" || evidenceText == "" {
		return false
	}
	return strings.Contains(
		transcript.NormalizeChinese(transcriptText),
		transcript.NormalizeChinese(evidenceText),
	)
}

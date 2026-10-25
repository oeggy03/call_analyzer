package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/oeggy03/call_analyzer/internal/domain"
)

const DefaultSupabaseURL = "https://zoiwsytnyrdifggwleru.supabase.co"

type PostgRESTClient struct {
	baseURL    *url.URL
	secret     string
	httpClient *http.Client
}

func NewPostgRESTClient(baseURL, secret string, httpClient *http.Client) (*PostgRESTClient, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errors.New("sync: Supabase secret is required")
	}
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("SUPABASE_URL"))
	}
	if baseURL == "" {
		baseURL = DefaultSupabaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("sync: invalid Supabase URL")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &PostgRESTClient{
		baseURL:    parsed,
		secret:     secret,
		httpClient: httpClient,
	}, nil
}

func (c *PostgRESTClient) PublishVocabularyEntry(
	ctx context.Context,
	entry domain.VocabularyEntry,
) error {
	if c == nil {
		return errors.New("sync: PostgREST client is not configured")
	}
	if strings.TrimSpace(entry.ID) == "" {
		return errors.New("sync: vocabulary entry id is required")
	}

	mergedIntoID := any(nil)
	if strings.TrimSpace(entry.MergedIntoID) != "" {
		mergedIntoID = entry.MergedIntoID
	}
	row := map[string]any{
		"id":             entry.ID,
		"simplified":     entry.Simplified,
		"traditional":    entry.Traditional,
		"reading":        entry.Reading,
		"marked_pinyin":  entry.MarkedPinyin,
		"status":         string(entry.Status),
		"provenance":     string(entry.Provenance),
		"confidence":     entry.Confidence,
		"teaching_cue":   entry.TeachingCue,
		"manual":         entry.Manual,
		"priority":       entry.Priority,
		"merged_into_id": mergedIntoID,
		"created_at":     entry.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":     entry.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	if err := c.requestJSON(
		ctx,
		http.MethodPost,
		"vocabulary_entries",
		url.Values{"on_conflict": []string{"id"}},
		[]map[string]any{row},
		"resolution=merge-duplicates,return=minimal",
		nil,
	); err != nil {
		return fmt.Errorf("sync: upsert vocabulary entry %s: %w", entry.ID, err)
	}

	tagIDs, err := c.upsertTags(ctx, entry.Tags)
	if err != nil {
		return fmt.Errorf("sync: upsert tags for %s: %w", entry.ID, err)
	}
	if err := c.replaceChildren(ctx, entry, tagIDs); err != nil {
		return fmt.Errorf("sync: replace vocabulary children for %s: %w", entry.ID, err)
	}

	if entry.Status == domain.VocabularyStatusConfirmed || entry.Manual {
		if err := c.requestJSON(
			ctx,
			http.MethodPost,
			"study_state",
			url.Values{"on_conflict": []string{"entry_id"}},
			[]map[string]any{{"entry_id": entry.ID}},
			"resolution=ignore-duplicates,return=minimal",
			nil,
		); err != nil {
			return fmt.Errorf("sync: initialize study state for %s: %w", entry.ID, err)
		}
	}
	return nil
}

func (c *PostgRESTClient) DeleteVocabularyEntry(ctx context.Context, entryID string) error {
	entryID = strings.TrimSpace(entryID)
	if c == nil {
		return errors.New("sync: PostgREST client is not configured")
	}
	if entryID == "" {
		return errors.New("sync: vocabulary entry id is required")
	}
	if err := c.requestJSON(
		ctx,
		http.MethodDelete,
		"vocabulary_entries",
		url.Values{"id": []string{"eq." + entryID}},
		nil,
		"",
		nil,
	); err != nil {
		return fmt.Errorf("sync: delete vocabulary entry %s: %w", entryID, err)
	}
	return nil
}

func (c *PostgRESTClient) upsertTags(
	ctx context.Context,
	tags []domain.Tag,
) (map[string]string, error) {
	if len(tags) == 0 {
		return map[string]string{}, nil
	}
	rows := make([]map[string]any, 0, len(tags))
	localIDs := make(map[string]string, len(tags))
	resolved := make(map[string]string, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		id := strings.TrimSpace(tag.ID)
		if id == "" {
			id = uuid.NewString()
		}
		localIDs[name] = id
		existingID, found, err := c.findTag(ctx, name)
		if err != nil {
			return nil, err
		}
		if found {
			resolved[name] = existingID
			continue
		}
		rows = append(rows, map[string]any{"id": id, "name": name})
	}
	if len(rows) > 0 {
		var returned []remoteTag
		if err := c.requestJSON(
			ctx,
			http.MethodPost,
			"tags",
			url.Values{"on_conflict": []string{"name"}},
			rows,
			"resolution=ignore-duplicates,return=representation",
			&returned,
		); err != nil {
			return nil, err
		}
		for _, tag := range returned {
			if name, id := strings.TrimSpace(tag.Name), strings.TrimSpace(tag.ID); name != "" && id != "" {
				resolved[name] = id
			}
		}
	}

	for name := range localIDs {
		if _, ok := resolved[name]; ok {
			continue
		}
		existingID, found, err := c.findTag(ctx, name)
		if err != nil {
			return nil, err
		}
		if found {
			resolved[name] = existingID
		}
	}
	for name, id := range localIDs {
		if _, ok := resolved[name]; !ok {
			resolved[name] = id
		}
	}
	return resolved, nil
}

type remoteTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *PostgRESTClient) findTag(
	ctx context.Context,
	name string,
) (string, bool, error) {
	var returned []remoteTag
	if err := c.requestJSON(
		ctx,
		http.MethodGet,
		"tags",
		url.Values{
			"name":   []string{"eq." + name},
			"select": []string{"id,name"},
		},
		nil,
		"",
		&returned,
	); err != nil {
		return "", false, err
	}
	if len(returned) == 0 {
		return "", false, nil
	}
	id := strings.TrimSpace(returned[0].ID)
	if id == "" {
		return "", false, nil
	}
	return id, true, nil
}

func (c *PostgRESTClient) replaceChildren(
	ctx context.Context,
	entry domain.VocabularyEntry,
	tagIDs map[string]string,
) error {
	for _, table := range []string{"entry_tags", "senses", "examples"} {
		if err := c.requestJSON(
			ctx,
			http.MethodDelete,
			table,
			url.Values{"entry_id": []string{"eq." + entry.ID}},
			nil,
			"",
			nil,
		); err != nil {
			return err
		}
	}

	senses := make([]map[string]any, 0, len(entry.Senses))
	for _, sense := range entry.Senses {
		senses = append(senses, map[string]any{
			"id":             sense.ID,
			"entry_id":       entry.ID,
			"gloss":          sense.Gloss,
			"part_of_speech": sense.PartOfSpeech,
			"classifier":     sense.Classifier,
			"sort_order":     sense.SortOrder,
		})
	}
	if err := c.insertRows(ctx, "senses", senses); err != nil {
		return err
	}

	examples := make([]map[string]any, 0, len(entry.Examples))
	for _, example := range entry.Examples {
		examples = append(examples, map[string]any{
			"id":          example.ID,
			"entry_id":    entry.ID,
			"simplified":  example.Simplified,
			"traditional": example.Traditional,
			"reading":     example.Reading,
			"translation": example.Translation,
			"provenance":  string(example.Provenance),
			"generated":   example.Generated,
		})
	}
	if err := c.insertRows(ctx, "examples", examples); err != nil {
		return err
	}

	entryTags := make([]map[string]any, 0, len(tagIDs))
	for _, tag := range entry.Tags {
		name := strings.TrimSpace(tag.Name)
		tagID := tagIDs[name]
		if name == "" || tagID == "" {
			continue
		}
		entryTags = append(entryTags, map[string]any{
			"entry_id": entry.ID,
			"tag_id":   tagID,
		})
	}
	return c.insertRows(ctx, "entry_tags", entryTags)
}

func (c *PostgRESTClient) insertRows(
	ctx context.Context,
	table string,
	rows []map[string]any,
) error {
	if len(rows) == 0 {
		return nil
	}
	return c.requestJSON(
		ctx,
		http.MethodPost,
		table,
		nil,
		rows,
		"return=minimal",
		nil,
	)
}

func (c *PostgRESTClient) requestJSON(
	ctx context.Context,
	method string,
	table string,
	query url.Values,
	payload any,
	prefer string,
	result any,
) error {
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/rest/v1/" + table
	endpoint.RawQuery = query.Encode()

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode %s: %w", table, err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return fmt.Errorf("create %s request: %w", table, err)
	}
	request.Header.Set("apikey", c.secret)
	request.Header.Set("Authorization", "Bearer "+c.secret)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if prefer != "" {
		request.Header.Set("Prefer", prefer)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send %s request: %w", table, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read %s response: %w", table, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(responseBody))
		message = strings.ReplaceAll(message, c.secret, "[redacted]")
		return fmt.Errorf("sync: Supabase HTTP %d: %s", response.StatusCode, message)
	}
	if result != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, result); err != nil {
			return fmt.Errorf("decode %s response: %w", table, err)
		}
	}
	return nil
}

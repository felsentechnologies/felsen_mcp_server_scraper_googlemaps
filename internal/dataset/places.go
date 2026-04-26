package dataset

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mcp_server_scraper_googlemaps/internal/models"
)

const (
	DefaultPlaceListLimit = 50
	MaxPlaceListLimit     = 500
)

var (
	ErrDatasetUnavailable = errors.New("dataset database is not enabled")
	ErrPlaceNotFound      = errors.New("dataset place not found")
)

type PlaceListFilter struct {
	Limit             int      `json:"limit,omitempty"`
	Offset            int      `json:"offset,omitempty"`
	Query             string   `json:"query,omitempty"`
	Search            string   `json:"search,omitempty"`
	Category          string   `json:"category,omitempty"`
	PlaceKey          string   `json:"placeKey,omitempty"`
	MinRating         *float64 `json:"minRating,omitempty"`
	MaxRating         *float64 `json:"maxRating,omitempty"`
	HasReviews        *bool    `json:"hasReviews,omitempty"`
	PendingActions    bool     `json:"pendingActions,omitempty"`
	ActionType        string   `json:"actionType,omitempty"`
	MissingActionType string   `json:"missingActionType,omitempty"`
}

type PlaceListResult struct {
	Count   int            `json:"count"`
	Total   int            `json:"total"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
	Results []DatasetPlace `json:"results"`
}

type DatasetPlace struct {
	ID            int64               `json:"id,omitempty"`
	ExtractionID  string              `json:"extractionId,omitempty"`
	ExtractedAt   *time.Time          `json:"extractedAt,omitempty"`
	PlaceKey      string              `json:"placeKey,omitempty"`
	GooglePlaceID *string             `json:"googlePlaceId,omitempty"`
	Actions       []models.ActionData `json:"actions"`
	Place         models.PlaceData    `json:"place"`
}

func (s *Store) ListPlaces(ctx context.Context, filter PlaceListFilter) (PlaceListResult, error) {
	if err := ctx.Err(); err != nil {
		return PlaceListResult{}, err
	}
	filter = normalizePlaceListFilter(filter)
	if s == nil || s.db == nil && s.path == "" {
		return PlaceListResult{}, ErrDatasetUnavailable
	}
	if s.db != nil {
		return s.listPostgresPlaces(ctx, filter)
	}
	return s.listFilePlaces(filter)
}

func (s *Store) GetPlace(ctx context.Context, id int64, key string) (*DatasetPlace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key = strings.TrimSpace(key)
	if id <= 0 && key == "" {
		return nil, errors.New("id or placeKey is required")
	}
	if s == nil || s.db == nil && s.path == "" {
		return nil, ErrDatasetUnavailable
	}
	if s.db != nil {
		return s.getPostgresPlace(ctx, id, key)
	}
	return s.getFilePlace(key)
}

func (s *Store) UpdatePlaceActions(ctx context.Context, id int64, key string, actions []models.ActionData) (*DatasetPlace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key = strings.TrimSpace(key)
	if id <= 0 && key == "" {
		return nil, errors.New("id or placeKey is required")
	}
	actions = defaultActions(actions)
	if err := validateActions(actions); err != nil {
		return nil, err
	}
	if s == nil || s.db == nil && s.path == "" {
		return nil, ErrDatasetUnavailable
	}
	if s.db != nil {
		return s.updatePostgresPlaceActions(ctx, id, key, actions)
	}
	return s.updateFilePlaceActions(key, actions)
}

func (s *Store) AppendPlaceAction(ctx context.Context, id int64, key string, action models.ActionData) (*DatasetPlace, error) {
	if action == nil {
		return nil, errors.New("action is required")
	}
	place, err := s.GetPlace(ctx, id, key)
	if err != nil {
		return nil, err
	}
	actions := append(defaultActions(place.Actions), action)
	return s.UpdatePlaceActions(ctx, id, place.PlaceKey, actions)
}

func normalizePlaceListFilter(filter PlaceListFilter) PlaceListFilter {
	if filter.Limit <= 0 {
		filter.Limit = DefaultPlaceListLimit
	}
	if filter.Limit > MaxPlaceListLimit {
		filter.Limit = MaxPlaceListLimit
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Search = strings.TrimSpace(filter.Search)
	filter.Category = strings.TrimSpace(filter.Category)
	filter.PlaceKey = strings.TrimSpace(filter.PlaceKey)
	filter.ActionType = strings.TrimSpace(filter.ActionType)
	filter.MissingActionType = strings.TrimSpace(filter.MissingActionType)
	return filter
}

func (s *Store) listPostgresPlaces(ctx context.Context, filter PlaceListFilter) (PlaceListResult, error) {
	where, args := placeFilterSQL(filter)
	countQuery := "SELECT count(*) FROM dataset_places WHERE " + where
	var total int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return PlaceListResult{}, fmt.Errorf("count dataset places: %w", err)
	}

	args = append(args, filter.Limit, filter.Offset)
	rows, err := s.db.QueryContext(ctx, datasetPlaceSelectSQL+" WHERE "+where+" ORDER BY id DESC LIMIT $"+fmt.Sprint(len(args)-1)+" OFFSET $"+fmt.Sprint(len(args)), args...)
	if err != nil {
		return PlaceListResult{}, fmt.Errorf("list dataset places: %w", err)
	}
	defer rows.Close()

	places := []DatasetPlace{}
	for rows.Next() {
		place, err := scanDatasetPlace(rows)
		if err != nil {
			return PlaceListResult{}, err
		}
		places = append(places, place)
	}
	if err := rows.Err(); err != nil {
		return PlaceListResult{}, fmt.Errorf("read dataset places: %w", err)
	}

	return PlaceListResult{
		Count:   len(places),
		Total:   total,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
		Results: places,
	}, nil
}

func (s *Store) getPostgresPlace(ctx context.Context, id int64, key string) (*DatasetPlace, error) {
	where := "id = $1"
	args := []any{id}
	if id <= 0 {
		where = "place_key = $1"
		args = []any{key}
	}

	row := s.db.QueryRowContext(ctx, datasetPlaceSelectSQL+" WHERE "+where, args...)
	place, err := scanDatasetPlace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPlaceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &place, nil
}

func (s *Store) updatePostgresPlaceActions(ctx context.Context, id int64, key string, actions []models.ActionData) (*DatasetPlace, error) {
	actionsJSON, err := marshalJSON(actions, "place actions")
	if err != nil {
		return nil, err
	}

	where := "id = $2"
	args := []any{actionsJSON, id}
	if id <= 0 {
		where = "place_key = $2"
		args = []any{actionsJSON, key}
	}

	row := s.db.QueryRowContext(ctx, `
UPDATE dataset_places
SET
	actions = $1::jsonb,
	raw_data = jsonb_set(raw_data, '{actions}', $1::jsonb, true)
WHERE `+where+`
RETURNING
	id,
	extraction_id,
	extracted_at,
	COALESCE(place_key, ''),
	google_place_id,
	query,
	name,
	address,
	phone,
	website,
	rating,
	reviews_count,
	category,
	google_maps_url,
	image_url,
	COALESCE(emails, '[]'::jsonb),
	COALESCE(phones, '[]'::jsonb),
	COALESCE(social_links, '{}'::jsonb),
	COALESCE(reviews, '[]'::jsonb),
	COALESCE(actions, '[]'::jsonb)
`, args...)
	place, err := scanDatasetPlace(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPlaceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &place, nil
}

func placeFilterSQL(filter PlaceListFilter) (string, []any) {
	clauses := []string{"TRUE"}
	args := []any{}
	addArg := func(value any) string {
		args = append(args, value)
		return "$" + fmt.Sprint(len(args))
	}

	if filter.Query != "" {
		clauses = append(clauses, "query ILIKE '%' || "+addArg(filter.Query)+" || '%'")
	}
	if filter.Search != "" {
		param := addArg(filter.Search)
		clauses = append(clauses, "(name ILIKE '%' || "+param+" || '%' OR COALESCE(address, '') ILIKE '%' || "+param+" || '%' OR COALESCE(website, '') ILIKE '%' || "+param+" || '%' OR COALESCE(phone, '') ILIKE '%' || "+param+" || '%')")
	}
	if filter.Category != "" {
		clauses = append(clauses, "category ILIKE '%' || "+addArg(filter.Category)+" || '%'")
	}
	if filter.PlaceKey != "" {
		clauses = append(clauses, "place_key = "+addArg(filter.PlaceKey))
	}
	if filter.MinRating != nil {
		clauses = append(clauses, "rating >= "+addArg(*filter.MinRating))
	}
	if filter.MaxRating != nil {
		clauses = append(clauses, "rating <= "+addArg(*filter.MaxRating))
	}
	if filter.HasReviews != nil {
		if *filter.HasReviews {
			clauses = append(clauses, "jsonb_array_length(reviews) > 0")
		} else {
			clauses = append(clauses, "jsonb_array_length(reviews) = 0")
		}
	}
	if filter.PendingActions {
		clauses = append(clauses, "jsonb_array_length(actions) = 0")
	}
	if filter.ActionType != "" {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM jsonb_array_elements(actions) action WHERE action->>'type' = "+addArg(filter.ActionType)+")")
	}
	if filter.MissingActionType != "" {
		clauses = append(clauses, "NOT EXISTS (SELECT 1 FROM jsonb_array_elements(actions) action WHERE action->>'type' = "+addArg(filter.MissingActionType)+")")
	}

	return strings.Join(clauses, " AND "), args
}

func (s *Store) listFilePlaces(filter PlaceListFilter) (PlaceListResult, error) {
	records, err := s.readFilePlaceRecords()
	if err != nil {
		return PlaceListResult{}, err
	}
	matches := []DatasetPlace{}
	for _, record := range records {
		place := fileRecordToDatasetPlace(record)
		if matchesPlaceFilter(place, filter) {
			matches = append(matches, place)
		}
	}
	total := len(matches)
	start := filter.Offset
	if start > total {
		start = total
	}
	end := start + filter.Limit
	if end > total {
		end = total
	}
	return PlaceListResult{
		Count:   end - start,
		Total:   total,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
		Results: matches[start:end],
	}, nil
}

func (s *Store) getFilePlace(key string) (*DatasetPlace, error) {
	if key == "" {
		return nil, errors.New("placeKey is required for file-backed dataset lookups")
	}
	records, err := s.readFilePlaceRecords()
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		place := fileRecordToDatasetPlace(record)
		if place.PlaceKey == key {
			return &place, nil
		}
	}
	return nil, ErrPlaceNotFound
}

func (s *Store) updateFilePlaceActions(key string, actions []models.ActionData) (*DatasetPlace, error) {
	if key == "" {
		return nil, errors.New("placeKey is required for file-backed dataset updates")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.readFilePlaceRecords()
	if err != nil {
		return nil, err
	}
	var updated *DatasetPlace
	for i := range records {
		if placeKey(records[i].Place) == key {
			records[i].Place.Actions = actions
			place := fileRecordToDatasetPlace(records[i])
			updated = &place
			break
		}
	}
	if updated == nil {
		return nil, ErrPlaceNotFound
	}
	if err := s.writeFilePlaceRecords(records); err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Store) readFilePlaceRecords() ([]PlaceRecord, error) {
	file, err := os.Open(filepath.Join(s.path, placesFileName))
	if os.IsNotExist(err) {
		return []PlaceRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open place records: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	records := []PlaceRecord{}
	for {
		var record PlaceRecord
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode place record: %w", err)
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *Store) writeFilePlaceRecords(records []PlaceRecord) error {
	if err := os.MkdirAll(s.path, 0o755); err != nil {
		return fmt.Errorf("create dataset directory: %w", err)
	}
	file, err := os.Create(filepath.Join(s.path, placesFileName))
	if err != nil {
		return fmt.Errorf("rewrite place records: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return fmt.Errorf("encode place record: %w", err)
		}
	}
	return nil
}

func fileRecordToDatasetPlace(record PlaceRecord) DatasetPlace {
	place := record.Place
	place.Actions = defaultActions(place.Actions)
	extractedAt := record.ExtractedAt
	return DatasetPlace{
		ExtractionID: record.ExtractionID,
		ExtractedAt:  &extractedAt,
		PlaceKey:     placeKey(place),
		Actions:      place.Actions,
		Place:        place,
	}
}

func matchesPlaceFilter(place DatasetPlace, filter PlaceListFilter) bool {
	if filter.Query != "" && !containsFold(place.Place.Query, filter.Query) {
		return false
	}
	if filter.Search != "" && !containsFold(place.Place.Name+" "+stringValue(place.Place.Address)+" "+stringValue(place.Place.Website)+" "+stringValue(place.Place.Phone), filter.Search) {
		return false
	}
	if filter.Category != "" && !containsFold(stringValue(place.Place.Category), filter.Category) {
		return false
	}
	if filter.PlaceKey != "" && place.PlaceKey != filter.PlaceKey {
		return false
	}
	if filter.MinRating != nil && (place.Place.Rating == nil || *place.Place.Rating < *filter.MinRating) {
		return false
	}
	if filter.MaxRating != nil && (place.Place.Rating == nil || *place.Place.Rating > *filter.MaxRating) {
		return false
	}
	if filter.HasReviews != nil && (*filter.HasReviews != (len(place.Place.Reviews) > 0)) {
		return false
	}
	if filter.PendingActions && len(place.Actions) > 0 {
		return false
	}
	if filter.ActionType != "" && !hasActionType(place.Actions, filter.ActionType) {
		return false
	}
	if filter.MissingActionType != "" && hasActionType(place.Actions, filter.MissingActionType) {
		return false
	}
	return true
}

func hasActionType(actions []models.ActionData, actionType string) bool {
	for _, action := range actions {
		if value, ok := action["type"].(string); ok && value == actionType {
			return true
		}
	}
	return false
}

func containsFold(value, needle string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(needle))
}

func validateActions(actions []models.ActionData) error {
	for i, action := range actions {
		if action == nil {
			return fmt.Errorf("actions[%d] must be an object", i)
		}
	}
	return nil
}

type datasetPlaceScanner interface {
	Scan(dest ...any) error
}

func scanDatasetPlace(scanner datasetPlaceScanner) (DatasetPlace, error) {
	var (
		place                        DatasetPlace
		extractedAt                  time.Time
		googlePlaceID                sql.NullString
		address, phone, website      sql.NullString
		rating                       sql.NullFloat64
		reviewsCount                 sql.NullInt64
		category, imageURL           sql.NullString
		emailsJSON, phonesJSON       []byte
		socialLinksJSON, reviewsJSON []byte
		actionsJSON                  []byte
	)
	if err := scanner.Scan(
		&place.ID,
		&place.ExtractionID,
		&extractedAt,
		&place.PlaceKey,
		&googlePlaceID,
		&place.Place.Query,
		&place.Place.Name,
		&address,
		&phone,
		&website,
		&rating,
		&reviewsCount,
		&category,
		&place.Place.GoogleMapsURL,
		&imageURL,
		&emailsJSON,
		&phonesJSON,
		&socialLinksJSON,
		&reviewsJSON,
		&actionsJSON,
	); err != nil {
		return DatasetPlace{}, err
	}

	place.ExtractedAt = &extractedAt
	place.GooglePlaceID = stringPtrFromNull(googlePlaceID)
	place.Place.Address = stringPtrFromNull(address)
	place.Place.Phone = stringPtrFromNull(phone)
	place.Place.Website = stringPtrFromNull(website)
	place.Place.Rating = floatPtrFromNull(rating)
	if reviewsCount.Valid {
		value := int(reviewsCount.Int64)
		place.Place.ReviewsCount = &value
	}
	place.Place.Category = stringPtrFromNull(category)
	place.Place.ImageURL = stringPtrFromNull(imageURL)

	if err := decodeJSONField(emailsJSON, &place.Place.Emails, "emails"); err != nil {
		return DatasetPlace{}, err
	}
	if err := decodeJSONField(phonesJSON, &place.Place.Phones, "phones"); err != nil {
		return DatasetPlace{}, err
	}
	if err := decodeJSONField(socialLinksJSON, &place.Place.SocialLinks, "socialLinks"); err != nil {
		return DatasetPlace{}, err
	}
	if err := decodeJSONField(reviewsJSON, &place.Place.Reviews, "reviews"); err != nil {
		return DatasetPlace{}, err
	}
	if err := decodeJSONField(actionsJSON, &place.Actions, "actions"); err != nil {
		return DatasetPlace{}, err
	}
	place.Actions = defaultActions(place.Actions)
	place.Place.Actions = place.Actions
	return place, nil
}

func decodeJSONField(payload []byte, target any, label string) error {
	if len(payload) == 0 {
		payload = []byte("null")
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("decode place %s: %w", label, err)
	}
	return nil
}

func stringPtrFromNull(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func floatPtrFromNull(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

const datasetPlaceSelectSQL = `
SELECT
	id,
	extraction_id,
	extracted_at,
	COALESCE(place_key, ''),
	google_place_id,
	query,
	name,
	address,
	phone,
	website,
	rating,
	reviews_count,
	category,
	google_maps_url,
	image_url,
	COALESCE(emails, '[]'::jsonb),
	COALESCE(phones, '[]'::jsonb),
	COALESCE(social_links, '{}'::jsonb),
	COALESCE(reviews, '[]'::jsonb),
	COALESCE(actions, '[]'::jsonb)
FROM dataset_places`

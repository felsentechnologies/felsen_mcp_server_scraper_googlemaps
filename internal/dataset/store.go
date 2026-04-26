package dataset

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"mcp_server_scraper_googlemaps/internal/models"
)

const (
	defaultPath    = "dataset"
	runsFileName   = "extractions.jsonl"
	placesFileName = "places.jsonl"
)

type Store struct {
	db     *sql.DB
	logger *log.Logger
	mu     sync.Mutex
	path   string
}

type ExtractionStatus string

const (
	ExtractionStatusRunning  ExtractionStatus = "running"
	ExtractionStatusFinished ExtractionStatus = "finished"
	ExtractionStatusFailed   ExtractionStatus = "failed"
	ExtractionStatusCanceled ExtractionStatus = "canceled"
)

type ExtractionRecord struct {
	ID          string             `json:"id"`
	ExtractedAt time.Time          `json:"extractedAt"`
	FinishedAt  *time.Time         `json:"finishedAt,omitempty"`
	Status      ExtractionStatus   `json:"status"`
	Error       *string            `json:"error,omitempty"`
	Input       models.Input       `json:"input"`
	Count       int                `json:"count"`
	Results     []models.PlaceData `json:"results"`
}

type PlaceRecord struct {
	ExtractionID string           `json:"extractionId"`
	ExtractedAt  time.Time        `json:"extractedAt"`
	Place        models.PlaceData `json:"place"`
}

type ExtractionSession struct {
	store    *Store
	record   ExtractionRecord
	seenKeys map[string]struct{}
	finished bool
}

func OpenFromEnv(ctx context.Context, logger *log.Logger) (*Store, error) {
	if logger == nil {
		logger = log.Default()
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return NewNoop(logger), nil
	}

	store, err := OpenPostgres(ctx, databaseURL, logger)
	if err != nil {
		return nil, err
	}
	logger.Printf("DATABASE_URL detected; dataset database is enabled")
	return store, nil
}

func NewNoop(logger *log.Logger) *Store {
	if logger == nil {
		logger = log.Default()
	}
	return &Store{logger: logger}
}

func New(path string, logger *log.Logger) *Store {
	if logger == nil {
		logger = log.Default()
	}
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultPath
	}
	return &Store{path: path, logger: logger}
}

func OpenPostgres(ctx context.Context, databaseURL string, logger *log.Logger) (*Store, error) {
	if logger == nil {
		logger = log.Default()
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open dataset database: %w", err)
	}
	store := &Store{db: db, logger: logger}

	migrationCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := store.Migrate(migrationCtx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect dataset database: %w", err)
	}

	const schema = `
CREATE TABLE IF NOT EXISTS dataset_extractions (
	id TEXT PRIMARY KEY,
	extracted_at TIMESTAMPTZ NOT NULL,
	finished_at TIMESTAMPTZ,
	status TEXT NOT NULL DEFAULT 'running',
	error TEXT,
	input JSONB NOT NULL,
	count INTEGER NOT NULL,
	results JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS dataset_places (
	id BIGSERIAL PRIMARY KEY,
	extraction_id TEXT NOT NULL REFERENCES dataset_extractions(id) ON DELETE CASCADE,
	extracted_at TIMESTAMPTZ NOT NULL,
	query TEXT NOT NULL,
	place_key TEXT,
	google_place_id TEXT,
	name TEXT NOT NULL,
	address TEXT,
	phone TEXT,
	website TEXT,
	rating DOUBLE PRECISION,
	reviews_count INTEGER,
	category TEXT,
	google_maps_url TEXT NOT NULL,
	image_url TEXT,
	emails JSONB NOT NULL DEFAULT '[]'::jsonb,
	phones JSONB NOT NULL DEFAULT '[]'::jsonb,
	social_links JSONB NOT NULL DEFAULT '{}'::jsonb,
	reviews JSONB NOT NULL DEFAULT '[]'::jsonb,
	actions JSONB NOT NULL DEFAULT '[]'::jsonb,
	raw_data JSONB NOT NULL
);

DO $$
BEGIN
	IF EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'dataset_places' AND column_name = 'data'
	) AND NOT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'dataset_places' AND column_name = 'raw_data'
	) THEN
		ALTER TABLE dataset_places RENAME COLUMN data TO raw_data;
	END IF;
END $$;

ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS google_place_id TEXT;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS place_key TEXT;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS address TEXT;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS phone TEXT;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS website TEXT;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS rating DOUBLE PRECISION;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS reviews_count INTEGER;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS category TEXT;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS image_url TEXT;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS emails JSONB;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS phones JSONB;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS social_links JSONB;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS reviews JSONB;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS actions JSONB;
ALTER TABLE dataset_places ADD COLUMN IF NOT EXISTS raw_data JSONB;
ALTER TABLE dataset_extractions ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ;
ALTER TABLE dataset_extractions ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'running';
ALTER TABLE dataset_extractions ADD COLUMN IF NOT EXISTS error TEXT;

UPDATE dataset_places
SET raw_data = '{}'::jsonb
WHERE raw_data IS NULL;

UPDATE dataset_places
SET
	google_place_id = COALESCE(google_place_id, substring(google_maps_url from '!1s([^!/?&#]+)')),
	address = COALESCE(address, NULLIF(raw_data->>'address', '')),
	phone = COALESCE(phone, NULLIF(raw_data->>'phone', '')),
	website = COALESCE(website, NULLIF(raw_data->>'website', '')),
	rating = COALESCE(
		rating,
		CASE
			WHEN raw_data->>'rating' ~ '^[0-9]+(\.[0-9]+)?$' THEN (raw_data->>'rating')::DOUBLE PRECISION
			ELSE NULL
		END
	),
	reviews_count = COALESCE(
		reviews_count,
		CASE
			WHEN raw_data->>'reviewsCount' ~ '^[0-9]+$' THEN (raw_data->>'reviewsCount')::INTEGER
			ELSE NULL
		END
	),
	category = COALESCE(category, NULLIF(raw_data->>'category', '')),
	image_url = COALESCE(image_url, NULLIF(raw_data->>'imageUrl', '')),
	emails = CASE WHEN jsonb_typeof(raw_data->'emails') = 'array' THEN raw_data->'emails' ELSE emails END,
	phones = CASE WHEN jsonb_typeof(raw_data->'phones') = 'array' THEN raw_data->'phones' ELSE phones END,
	social_links = CASE WHEN jsonb_typeof(raw_data->'socialLinks') = 'object' THEN raw_data->'socialLinks' ELSE social_links END,
	reviews = CASE WHEN jsonb_typeof(raw_data->'reviews') = 'array' THEN raw_data->'reviews' ELSE reviews END
WHERE raw_data IS NOT NULL;

UPDATE dataset_places
SET place_key = COALESCE(
	CASE
		WHEN NULLIF(btrim(google_place_id), '') IS NOT NULL
		THEN 'google_place_id:' || lower(btrim(google_place_id))
	END,
	CASE
		WHEN NULLIF(btrim(name), '') IS NOT NULL AND NULLIF(btrim(address), '') IS NOT NULL
		THEN 'name_address:' || lower(regexp_replace(btrim(name), '[[:space:]]+', ' ', 'g')) || '|' || lower(regexp_replace(btrim(address), '[[:space:]]+', ' ', 'g'))
	END,
	CASE
		WHEN NULLIF(btrim(name), '') IS NOT NULL AND NULLIF(btrim(phone), '') IS NOT NULL
		THEN 'name_phone:' || lower(regexp_replace(btrim(name), '[[:space:]]+', ' ', 'g')) || '|' || lower(regexp_replace(btrim(phone), '[[:space:]]+', ' ', 'g'))
	END,
	CASE
		WHEN NULLIF(btrim(name), '') IS NOT NULL AND NULLIF(btrim(website), '') IS NOT NULL
		THEN 'name_website:' || lower(regexp_replace(btrim(name), '[[:space:]]+', ' ', 'g')) || '|' || lower(regexp_replace(split_part(split_part(btrim(website), '?', 1), '#', 1), '/+$', ''))
	END,
	CASE
		WHEN NULLIF(btrim(google_maps_url), '') IS NOT NULL
		THEN 'google_maps_url:' || lower(regexp_replace(split_part(split_part(btrim(google_maps_url), '?', 1), '#', 1), '/+$', ''))
	END
)
WHERE place_key IS NULL OR place_key = '';

UPDATE dataset_places SET emails = '[]'::jsonb WHERE emails IS NULL;
UPDATE dataset_places SET phones = '[]'::jsonb WHERE phones IS NULL;
UPDATE dataset_places SET social_links = '{}'::jsonb WHERE social_links IS NULL;
UPDATE dataset_places SET reviews = '[]'::jsonb WHERE reviews IS NULL;
UPDATE dataset_places SET actions = '[]'::jsonb WHERE actions IS NULL;

ALTER TABLE dataset_places ALTER COLUMN raw_data SET NOT NULL;
ALTER TABLE dataset_places ALTER COLUMN emails SET DEFAULT '[]'::jsonb;
ALTER TABLE dataset_places ALTER COLUMN emails SET NOT NULL;
ALTER TABLE dataset_places ALTER COLUMN phones SET DEFAULT '[]'::jsonb;
ALTER TABLE dataset_places ALTER COLUMN phones SET NOT NULL;
ALTER TABLE dataset_places ALTER COLUMN social_links SET DEFAULT '{}'::jsonb;
ALTER TABLE dataset_places ALTER COLUMN social_links SET NOT NULL;
ALTER TABLE dataset_places ALTER COLUMN reviews SET DEFAULT '[]'::jsonb;
ALTER TABLE dataset_places ALTER COLUMN reviews SET NOT NULL;
ALTER TABLE dataset_places ALTER COLUMN actions SET DEFAULT '[]'::jsonb;
ALTER TABLE dataset_places ALTER COLUMN actions SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_dataset_places_extraction_id ON dataset_places(extraction_id);
CREATE INDEX IF NOT EXISTS idx_dataset_places_place_key ON dataset_places(place_key) WHERE place_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_dataset_places_query ON dataset_places(query);
CREATE INDEX IF NOT EXISTS idx_dataset_places_google_place_id ON dataset_places(google_place_id) WHERE google_place_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_dataset_places_name_lower ON dataset_places(lower(name));
CREATE INDEX IF NOT EXISTS idx_dataset_places_phone ON dataset_places(phone);
CREATE INDEX IF NOT EXISTS idx_dataset_places_website ON dataset_places(website);
CREATE INDEX IF NOT EXISTS idx_dataset_places_category ON dataset_places(category);
CREATE INDEX IF NOT EXISTS idx_dataset_places_rating ON dataset_places(rating);
CREATE INDEX IF NOT EXISTS idx_dataset_places_reviews_count ON dataset_places(reviews_count);
CREATE INDEX IF NOT EXISTS idx_dataset_places_google_maps_url ON dataset_places(google_maps_url);
CREATE INDEX IF NOT EXISTS idx_dataset_places_emails_gin ON dataset_places USING GIN(emails);
CREATE INDEX IF NOT EXISTS idx_dataset_places_phones_gin ON dataset_places USING GIN(phones);
CREATE INDEX IF NOT EXISTS idx_dataset_places_actions_gin ON dataset_places USING GIN(actions);
CREATE INDEX IF NOT EXISTS idx_dataset_places_raw_data_gin ON dataset_places USING GIN(raw_data);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate dataset database: %w", err)
	}
	return nil
}

func (s *Store) SaveExtraction(ctx context.Context, input models.Input, results []models.PlaceData) error {
	session, err := s.StartExtraction(ctx, input)
	if err != nil {
		return err
	}
	for _, place := range results {
		if _, err := session.SavePlace(ctx, place); err != nil {
			return err
		}
	}
	return session.Finish(ctx)
}

func (s *Store) StartExtraction(ctx context.Context, input models.Input) (*ExtractionSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	session := &ExtractionSession{
		store: s,
		record: ExtractionRecord{
			ID:          newID(now),
			ExtractedAt: now,
			Status:      ExtractionStatusRunning,
			Input:       input,
			Results:     []models.PlaceData{},
		},
		seenKeys: make(map[string]struct{}),
	}
	if s == nil {
		return session, nil
	}

	if s.db != nil {
		if err := s.createPostgresExtraction(ctx, session.record); err != nil {
			return nil, err
		}
		return session, nil
	}
	if s.path != "" {
		s.mu.Lock()
		err := s.loadKnownPlaceKeys(session.seenKeys)
		s.mu.Unlock()
		if err != nil {
			return nil, err
		}
	}
	return session, nil
}

func (s *Store) createPostgresExtraction(ctx context.Context, record ExtractionRecord) error {
	inputJSON, err := json.Marshal(record.Input)
	if err != nil {
		return fmt.Errorf("encode extraction input: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO dataset_extractions (id, extracted_at, finished_at, status, error, input, count, results)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8::jsonb)
`, record.ID, record.ExtractedAt, nullableTime(record.FinishedAt), record.Status, nullableStringPtr(record.Error), string(inputJSON), 0, "[]"); err != nil {
		return fmt.Errorf("insert extraction record: %w", err)
	}
	return nil
}

func (e *ExtractionSession) SavePlace(ctx context.Context, place models.PlaceData) (bool, error) {
	if e == nil {
		return true, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	key := placeKey(place)
	if e.isSeen(key) {
		return false, nil
	}

	if e.store == nil || e.store.path == "" && e.store.db == nil {
		e.markSeen(key)
		e.addSavedPlace(place)
		return true, nil
	}
	if e.store.db != nil {
		saved, err := e.savePostgresPlace(ctx, place)
		if err != nil {
			return false, err
		}
		e.markSeen(key)
		if saved {
			e.addSavedPlace(place)
		}
		return saved, nil
	}

	e.store.mu.Lock()
	defer e.store.mu.Unlock()

	if err := os.MkdirAll(e.store.path, 0o755); err != nil {
		return false, fmt.Errorf("create dataset directory: %w", err)
	}
	placeRecord := PlaceRecord{
		ExtractionID: e.record.ID,
		ExtractedAt:  e.record.ExtractedAt,
		Place:        place,
	}
	if err := appendJSONLine(filepath.Join(e.store.path, placesFileName), placeRecord); err != nil {
		return false, fmt.Errorf("save place record: %w", err)
	}
	e.markSeen(key)
	e.addSavedPlace(place)
	return true, nil
}

func (e *ExtractionSession) Finish(ctx context.Context) error {
	return e.FinishWithStatus(ctx, ExtractionStatusFinished, "")
}

func (e *ExtractionSession) FinishWithStatus(ctx context.Context, status ExtractionStatus, errorMessage string) error {
	if e == nil || e.finished {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	e.finishRecord(status, errorMessage)
	if e.store == nil || e.store.path == "" && e.store.db == nil {
		e.finished = true
		return nil
	}
	if e.store.db != nil {
		resultsJSON, err := marshalJSON(e.record.Results, "extraction results")
		if err != nil {
			return err
		}
		if _, err := e.store.db.ExecContext(ctx, `
UPDATE dataset_extractions
SET count = $2, results = $3::jsonb, status = $4, finished_at = $5, error = $6
WHERE id = $1
`, e.record.ID, e.record.Count, resultsJSON, e.record.Status, nullableTime(e.record.FinishedAt), nullableStringPtr(e.record.Error)); err != nil {
			return fmt.Errorf("update extraction record: %w", err)
		}
		e.finished = true
		e.store.logger.Printf("saved extraction %s with status %s and %d unique result(s) to dataset database", e.record.ID, e.record.Status, e.record.Count)
		return nil
	}

	e.store.mu.Lock()
	defer e.store.mu.Unlock()

	if err := os.MkdirAll(e.store.path, 0o755); err != nil {
		return fmt.Errorf("create dataset directory: %w", err)
	}
	if err := appendJSONLine(filepath.Join(e.store.path, runsFileName), e.record); err != nil {
		return fmt.Errorf("save extraction record: %w", err)
	}
	e.finished = true
	e.store.logger.Printf("saved extraction %s with status %s and %d unique result(s) to %s", e.record.ID, e.record.Status, e.record.Count, e.store.path)
	return nil
}

func (e *ExtractionSession) finishRecord(status ExtractionStatus, errorMessage string) {
	if status == "" {
		status = ExtractionStatusFinished
	}
	finishedAt := time.Now().UTC()
	e.record.Status = status
	e.record.FinishedAt = &finishedAt
	if errorMessage = strings.TrimSpace(errorMessage); errorMessage != "" {
		e.record.Error = &errorMessage
	} else {
		e.record.Error = nil
	}
}

func (e *ExtractionSession) savePostgresPlace(ctx context.Context, place models.PlaceData) (saved bool, err error) {
	columns, err := newPlaceColumns(place)
	if err != nil {
		return false, err
	}

	tx, err := e.store.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin dataset place transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if columns.PlaceKey != "" {
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, columns.PlaceKey); err != nil {
			return false, fmt.Errorf("lock place identity: %w", err)
		}

		var exists bool
		if err = tx.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1 FROM dataset_places WHERE place_key = $1
)
`, columns.PlaceKey).Scan(&exists); err != nil {
			return false, fmt.Errorf("check duplicate place: %w", err)
		}
		if exists {
			if err = tx.Commit(); err != nil {
				return false, fmt.Errorf("commit duplicate place check: %w", err)
			}
			return false, nil
		}
	}

	if _, err = tx.ExecContext(ctx, `
INSERT INTO dataset_places (
	extraction_id,
	extracted_at,
	query,
	place_key,
	google_place_id,
	name,
	address,
	phone,
	website,
	rating,
	reviews_count,
	category,
	google_maps_url,
	image_url,
	emails,
	phones,
	social_links,
	reviews,
	raw_data
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
`, e.record.ID,
		e.record.ExtractedAt,
		place.Query,
		nullableString(columns.PlaceKey),
		columns.GooglePlaceID,
		place.Name,
		place.Address,
		place.Phone,
		place.Website,
		place.Rating,
		place.ReviewsCount,
		place.Category,
		place.GoogleMapsURL,
		place.ImageURL,
		columns.EmailsJSON,
		columns.PhonesJSON,
		columns.SocialLinksJSON,
		columns.ReviewsJSON,
		columns.RawDataJSON,
	); err != nil {
		return false, fmt.Errorf("insert place record: %w", err)
	}

	resultJSON, err := marshalJSON([]models.PlaceData{place}, "incremental extraction result")
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `
UPDATE dataset_extractions
SET count = count + 1, results = results || $2::jsonb
WHERE id = $1
`, e.record.ID, resultJSON)
	if err != nil {
		return false, fmt.Errorf("update extraction result: %w", err)
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr == nil && rows == 0 {
		err = fmt.Errorf("extraction %s does not exist", e.record.ID)
		return false, err
	}

	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("commit dataset place transaction: %w", err)
	}
	return true, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func appendJSONLine(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(value)
}

func (e *ExtractionSession) isSeen(key string) bool {
	if key == "" {
		return false
	}
	_, ok := e.seenKeys[key]
	return ok
}

func (e *ExtractionSession) markSeen(key string) {
	if key != "" {
		e.seenKeys[key] = struct{}{}
	}
}

func (e *ExtractionSession) addSavedPlace(place models.PlaceData) {
	e.record.Results = append(e.record.Results, place)
	e.record.Count = len(e.record.Results)
}

func (s *Store) loadKnownPlaceKeys(keys map[string]struct{}) error {
	file, err := os.Open(filepath.Join(s.path, placesFileName))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open existing place records: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	for {
		var record PlaceRecord
		if err := decoder.Decode(&record); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decode existing place record: %w", err)
		}
		if key := placeKey(record.Place); key != "" {
			keys[key] = struct{}{}
		}
	}
	return nil
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullableStringPtr(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: *value != ""}
}

func nullableTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *value, Valid: true}
}

type placeColumns struct {
	PlaceKey        string
	GooglePlaceID   *string
	EmailsJSON      string
	PhonesJSON      string
	SocialLinksJSON string
	ReviewsJSON     string
	RawDataJSON     string
}

func newPlaceColumns(place models.PlaceData) (placeColumns, error) {
	emailsJSON, err := marshalJSON(defaultStrings(place.Emails), "place emails")
	if err != nil {
		return placeColumns{}, err
	}
	phonesJSON, err := marshalJSON(defaultStrings(place.Phones), "place phones")
	if err != nil {
		return placeColumns{}, err
	}
	socialLinksJSON, err := marshalJSON(defaultSocialLinks(place.SocialLinks), "place social links")
	if err != nil {
		return placeColumns{}, err
	}
	reviewsJSON, err := marshalJSON(defaultReviews(place.Reviews), "place reviews")
	if err != nil {
		return placeColumns{}, err
	}
	rawDataJSON, err := marshalJSON(place, "place raw data")
	if err != nil {
		return placeColumns{}, err
	}

	return placeColumns{
		PlaceKey:        placeKey(place),
		GooglePlaceID:   googlePlaceID(place.GoogleMapsURL),
		EmailsJSON:      emailsJSON,
		PhonesJSON:      phonesJSON,
		SocialLinksJSON: socialLinksJSON,
		ReviewsJSON:     reviewsJSON,
		RawDataJSON:     rawDataJSON,
	}, nil
}

func marshalJSON(value any, label string) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", label, err)
	}
	return string(payload), nil
}

func defaultStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func defaultReviews(values []models.ReviewData) []models.ReviewData {
	if values == nil {
		return []models.ReviewData{}
	}
	return values
}

func defaultSocialLinks(value models.SocialLinks) models.SocialLinks {
	if value == nil {
		return models.SocialLinks{}
	}
	return value
}

func placeKey(place models.PlaceData) string {
	if googleID := googlePlaceID(place.GoogleMapsURL); googleID != nil {
		if key := identityKey("google_place_id", *googleID); key != "" {
			return key
		}
	}

	name := normalizeIdentityValue(place.Name)
	if name != "" {
		if address := normalizeIdentityValue(stringValue(place.Address)); address != "" {
			return "name_address:" + name + "|" + address
		}
		if phone := normalizeIdentityValue(stringValue(place.Phone)); phone != "" {
			return "name_phone:" + name + "|" + phone
		}
		if website := normalizeGoogleMapsURL(stringValue(place.Website)); website != "" {
			return "name_website:" + name + "|" + website
		}
	}

	if mapsURL := normalizeGoogleMapsURL(place.GoogleMapsURL); mapsURL != "" {
		return "google_maps_url:" + mapsURL
	}
	return ""
}

func identityKey(prefix, value string) string {
	value = normalizeIdentityValue(value)
	if value == "" {
		return ""
	}
	return prefix + ":" + value
}

func normalizeIdentityValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Join(strings.Fields(value), " ")
}

func normalizeGoogleMapsURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return normalizeIdentityValue(rawURL)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.ToLower(parsed.String())
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func googlePlaceID(rawURL string) *string {
	if rawURL == "" {
		return nil
	}
	if match := googlePlaceIDFromURLRegex.FindStringSubmatch(rawURL); len(match) == 2 {
		if decoded, err := url.PathUnescape(match[1]); err == nil && decoded != "" {
			return &decoded
		}
		return &match[1]
	}
	if parsed, err := url.Parse(rawURL); err == nil {
		for _, key := range []string{"cid", "ftid"} {
			if value := strings.TrimSpace(parsed.Query().Get(key)); value != "" {
				return &value
			}
		}
	}
	if match := googlePlaceCIDRegex.FindStringSubmatch(rawURL); len(match) == 2 {
		if decoded, err := url.QueryUnescape(match[1]); err == nil && decoded != "" {
			return &decoded
		}
		return &match[1]
	}
	return nil
}

func newID(now time.Time) string {
	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return now.Format("20060102T150405.000000000Z")
	}
	return now.Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(suffix[:])
}

var (
	googlePlaceIDFromURLRegex = regexp.MustCompile(`!1s([^!/?&#]+)`)
	googlePlaceCIDRegex       = regexp.MustCompile(`[?&]cid=([^&#]+)`)
)

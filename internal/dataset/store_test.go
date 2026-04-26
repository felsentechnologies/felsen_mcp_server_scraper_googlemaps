package dataset

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mcp_server_scraper_googlemaps/internal/models"
)

func TestSaveExtractionWritesRunAndPlaces(t *testing.T) {
	store := New(t.TempDir(), nil)
	rating := 4.8
	results := []models.PlaceData{
		{
			Query:         "pizzarias em Curitiba",
			Name:          "Piola Curitiba",
			Rating:        &rating,
			GoogleMapsURL: "https://www.google.com/maps/place/Piola+Curitiba",
			Emails:        []string{},
			Phones:        []string{"(41) 98516-9206"},
			SocialLinks:   models.EmptySocialLinks(),
		},
	}

	err := store.SaveExtraction(context.Background(), models.Input{
		SearchQueries:     []string{"pizzarias em Curitiba"},
		MaxPlacesPerQuery: 1,
	}, results)
	if err != nil {
		t.Fatalf("SaveExtraction() error = %v", err)
	}

	run := readOneJSONLine[ExtractionRecord](t, filepath.Join(store.path, runsFileName))
	if run.ID == "" {
		t.Fatal("run.ID is empty")
	}
	if run.Count != 1 || len(run.Results) != 1 {
		t.Fatalf("run = %#v, want one result", run)
	}
	if run.Status != ExtractionStatusFinished {
		t.Fatalf("run.Status = %q, want %q", run.Status, ExtractionStatusFinished)
	}
	if run.FinishedAt == nil {
		t.Fatal("run.FinishedAt is nil")
	}
	if run.Results[0].Name != "Piola Curitiba" {
		t.Fatalf("run.Results[0].Name = %q", run.Results[0].Name)
	}

	place := readOneJSONLine[PlaceRecord](t, filepath.Join(store.path, placesFileName))
	if place.ExtractionID != run.ID {
		t.Fatalf("place.ExtractionID = %q, want %q", place.ExtractionID, run.ID)
	}
	if place.Place.GoogleMapsURL != results[0].GoogleMapsURL {
		t.Fatalf("place URL = %q", place.Place.GoogleMapsURL)
	}
}

func TestExtractionSessionWritesPlaceBeforeFinishAndSkipsDuplicates(t *testing.T) {
	store := New(t.TempDir(), nil)
	ctx := context.Background()
	address := "Rua Emiliano Perneta, 680"
	first := models.PlaceData{
		Query:         "pizzarias em Curitiba",
		Name:          "Avenida Paulista - Pizza",
		Address:       &address,
		GoogleMapsURL: "https://www.google.com/maps/place/avenida-paulista",
		Emails:        []string{},
		Phones:        []string{},
		SocialLinks:   models.EmptySocialLinks(),
	}

	session, err := store.StartExtraction(ctx, models.Input{
		SearchQueries:     []string{"pizzarias em Curitiba"},
		MaxPlacesPerQuery: 2,
	})
	if err != nil {
		t.Fatalf("StartExtraction() error = %v", err)
	}
	saved, err := session.SavePlace(ctx, first)
	if err != nil {
		t.Fatalf("SavePlace(first) error = %v", err)
	}
	if !saved {
		t.Fatal("SavePlace(first) saved = false, want true")
	}

	place := readOneJSONLine[PlaceRecord](t, filepath.Join(store.path, placesFileName))
	if place.ExtractionID != session.record.ID {
		t.Fatalf("place.ExtractionID = %q, want %q", place.ExtractionID, session.record.ID)
	}

	duplicate := first
	duplicate.Query = "pizza centro curitiba"
	duplicate.GoogleMapsURL = "https://www.google.com/maps/place/avenida-paulista?entry=ttu"
	saved, err = session.SavePlace(ctx, duplicate)
	if err != nil {
		t.Fatalf("SavePlace(duplicate) error = %v", err)
	}
	if saved {
		t.Fatal("SavePlace(duplicate) saved = true, want false")
	}

	if err := session.Finish(ctx); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	run := readOneJSONLine[ExtractionRecord](t, filepath.Join(store.path, runsFileName))
	if run.Count != 1 || len(run.Results) != 1 {
		t.Fatalf("run = %#v, want one unique result", run)
	}
	if run.Status != ExtractionStatusFinished {
		t.Fatalf("run.Status = %q, want %q", run.Status, ExtractionStatusFinished)
	}
	places := readJSONLines[PlaceRecord](t, filepath.Join(store.path, placesFileName))
	if len(places) != 1 {
		t.Fatalf("len(places) = %d, want 1", len(places))
	}
}

func TestExtractionSessionWritesFailedStatusWithPartialResults(t *testing.T) {
	store := New(t.TempDir(), nil)
	ctx := context.Background()
	first := models.PlaceData{
		Query:         "pizzarias em Curitiba",
		Name:          "Avenida Paulista - Pizza",
		GoogleMapsURL: "https://www.google.com/maps/place/avenida-paulista",
		Emails:        []string{},
		Phones:        []string{},
		SocialLinks:   models.EmptySocialLinks(),
	}

	session, err := store.StartExtraction(ctx, models.Input{
		SearchQueries:     []string{"pizzarias em Curitiba"},
		MaxPlacesPerQuery: 2,
	})
	if err != nil {
		t.Fatalf("StartExtraction() error = %v", err)
	}
	if _, err := session.SavePlace(ctx, first); err != nil {
		t.Fatalf("SavePlace() error = %v", err)
	}
	if err := session.FinishWithStatus(ctx, ExtractionStatusFailed, "collect place urls: timeout"); err != nil {
		t.Fatalf("FinishWithStatus() error = %v", err)
	}

	run := readOneJSONLine[ExtractionRecord](t, filepath.Join(store.path, runsFileName))
	if run.Status != ExtractionStatusFailed {
		t.Fatalf("run.Status = %q, want %q", run.Status, ExtractionStatusFailed)
	}
	if run.Error == nil || *run.Error != "collect place urls: timeout" {
		t.Fatalf("run.Error = %#v, want failure message", run.Error)
	}
	if run.FinishedAt == nil {
		t.Fatal("run.FinishedAt is nil")
	}
	if run.Count != 1 || len(run.Results) != 1 {
		t.Fatalf("run = %#v, want one partial result", run)
	}
}

func TestExtractionSessionWritesCanceledStatus(t *testing.T) {
	store := New(t.TempDir(), nil)
	ctx := context.Background()

	session, err := store.StartExtraction(ctx, models.Input{
		SearchQueries:     []string{"pizzarias em Curitiba"},
		MaxPlacesPerQuery: 2,
	})
	if err != nil {
		t.Fatalf("StartExtraction() error = %v", err)
	}
	if err := session.FinishWithStatus(ctx, ExtractionStatusCanceled, ""); err != nil {
		t.Fatalf("FinishWithStatus() error = %v", err)
	}

	run := readOneJSONLine[ExtractionRecord](t, filepath.Join(store.path, runsFileName))
	if run.Status != ExtractionStatusCanceled {
		t.Fatalf("run.Status = %q, want %q", run.Status, ExtractionStatusCanceled)
	}
	if run.Error != nil {
		t.Fatalf("run.Error = %#v, want nil", run.Error)
	}
	if run.FinishedAt == nil {
		t.Fatal("run.FinishedAt is nil")
	}
}

func TestPlaceKeyUsesStableIdentityAcrossURLVariants(t *testing.T) {
	first := models.PlaceData{
		Name:          "Avenida Paulista - Pizza",
		Address:       stringPtrForTest("Rua Emiliano Perneta, 680"),
		GoogleMapsURL: "https://www.google.com/maps/place/avenida-paulista",
	}
	second := models.PlaceData{
		Name:          "  avenida paulista - pizza  ",
		Address:       stringPtrForTest("Rua Emiliano Perneta,   680"),
		GoogleMapsURL: "https://www.google.com/maps/place/avenida-paulista?entry=ttu",
	}

	if placeKey(first) != placeKey(second) {
		t.Fatalf("placeKey mismatch: %q != %q", placeKey(first), placeKey(second))
	}
}

func TestNewPlaceColumnsPreservesRawDataAndStructuredJSON(t *testing.T) {
	rating := 4.7
	reviewsCount := 123
	instagram := "https://instagram.com/example"
	publishedAt := "2 semanas atras"
	place := models.PlaceData{
		Query:         "pizzarias em Curitiba",
		Name:          "Avenida Paulista - Pizza",
		Address:       stringPtrForTest("Rua Emiliano Perneta, 680"),
		Phone:         stringPtrForTest("(41) 3322-1441"),
		Website:       stringPtrForTest("https://example.com"),
		Rating:        &rating,
		ReviewsCount:  &reviewsCount,
		Category:      stringPtrForTest("Pizzaria"),
		GoogleMapsURL: "https://www.google.com/maps/place/example/data=!4m6!3m5!1s0x94dce46f1234567%3A0xabcdef1234567890!8m2",
		ImageURL:      stringPtrForTest("https://lh3.googleusercontent.com/photo.jpg"),
		Emails:        []string{"contato@example.com"},
		Phones:        []string{"(41) 3322-1441"},
		SocialLinks: models.SocialLinks{
			"instagram": &instagram,
		},
		Reviews: []models.ReviewData{
			{Author: "Cliente", Rating: &rating, PublishedAt: &publishedAt, Text: "Otimo atendimento."},
		},
		Actions: []models.ActionData{
			{"type": "contact", "status": "pending"},
		},
	}

	columns, err := newPlaceColumns(place)
	if err != nil {
		t.Fatalf("newPlaceColumns() error = %v", err)
	}
	if columns.GooglePlaceID == nil || *columns.GooglePlaceID != "0x94dce46f1234567:0xabcdef1234567890" {
		t.Fatalf("GooglePlaceID = %#v", columns.GooglePlaceID)
	}
	if columns.PlaceKey != "google_place_id:0x94dce46f1234567:0xabcdef1234567890" {
		t.Fatalf("PlaceKey = %q", columns.PlaceKey)
	}

	var raw models.PlaceData
	if err := json.Unmarshal([]byte(columns.RawDataJSON), &raw); err != nil {
		t.Fatalf("decode raw data: %v", err)
	}
	if raw.Name != place.Name || raw.GoogleMapsURL != place.GoogleMapsURL {
		t.Fatalf("raw = %#v, want original place fields", raw)
	}

	var emails []string
	if err := json.Unmarshal([]byte(columns.EmailsJSON), &emails); err != nil {
		t.Fatalf("decode emails: %v", err)
	}
	if len(emails) != 1 || emails[0] != "contato@example.com" {
		t.Fatalf("emails = %#v", emails)
	}

	var reviews []models.ReviewData
	if err := json.Unmarshal([]byte(columns.ReviewsJSON), &reviews); err != nil {
		t.Fatalf("decode reviews: %v", err)
	}
	if len(reviews) != 1 || reviews[0].Author != "Cliente" {
		t.Fatalf("reviews = %#v", reviews)
	}

	var actions []models.ActionData
	if err := json.Unmarshal([]byte(columns.ActionsJSON), &actions); err != nil {
		t.Fatalf("decode actions: %v", err)
	}
	if len(actions) != 1 || actions[0]["type"] != "contact" {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestNewPlaceColumnsDefaultsNilCollections(t *testing.T) {
	columns, err := newPlaceColumns(models.PlaceData{
		Name:          "Sem contatos",
		GoogleMapsURL: "https://www.google.com/maps/place/Sem+Contatos",
	})
	if err != nil {
		t.Fatalf("newPlaceColumns() error = %v", err)
	}

	for label, got := range map[string]string{
		"emails":  columns.EmailsJSON,
		"phones":  columns.PhonesJSON,
		"reviews": columns.ReviewsJSON,
		"actions": columns.ActionsJSON,
	} {
		if got != "[]" {
			t.Fatalf("%s JSON = %s, want []", label, got)
		}
	}
	if columns.SocialLinksJSON != "{}" {
		t.Fatalf("social links JSON = %s, want {}", columns.SocialLinksJSON)
	}
	if columns.GooglePlaceID != nil {
		t.Fatalf("GooglePlaceID = %#v, want nil", columns.GooglePlaceID)
	}
}

func TestOpenFromEnvReturnsNoopWithoutDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	store, err := OpenFromEnv(context.Background(), nil)
	if err != nil {
		t.Fatalf("OpenFromEnv() error = %v", err)
	}
	if store == nil {
		t.Fatal("OpenFromEnv() returned nil store")
	}
	if err := store.SaveExtraction(context.Background(), models.Input{}, nil); err != nil {
		t.Fatalf("SaveExtraction() error = %v", err)
	}
}

func TestListPlacesAndUpdateActionsFileStore(t *testing.T) {
	store := New(t.TempDir(), nil)
	ctx := context.Background()
	rating := 4.9
	category := "Pizzaria"
	publishedAt := "1 semana atras"
	first := models.PlaceData{
		Query:         "pizzarias em Curitiba",
		Name:          "Pizza Central",
		Address:       stringPtrForTest("Rua A, 123"),
		Rating:        &rating,
		Category:      &category,
		GoogleMapsURL: "https://www.google.com/maps/place/pizza-central",
		Emails:        []string{},
		Phones:        []string{},
		SocialLinks:   models.EmptySocialLinks(),
		Reviews: []models.ReviewData{
			{Author: "Cliente", PublishedAt: &publishedAt, Text: "Muito bom."},
		},
	}
	second := models.PlaceData{
		Query:         "cafes em Curitiba",
		Name:          "Cafe Central",
		Address:       stringPtrForTest("Rua B, 456"),
		GoogleMapsURL: "https://www.google.com/maps/place/cafe-central",
		Emails:        []string{},
		Phones:        []string{},
		SocialLinks:   models.EmptySocialLinks(),
		Actions: []models.ActionData{
			{"type": "contact", "status": "done"},
		},
	}

	if err := store.SaveExtraction(ctx, models.Input{SearchQueries: []string{"pizzarias em Curitiba"}}, []models.PlaceData{first, second}); err != nil {
		t.Fatalf("SaveExtraction() error = %v", err)
	}

	hasReviews := true
	result, err := store.ListPlaces(ctx, PlaceListFilter{
		Category:       "pizz",
		MinRating:      &rating,
		HasReviews:     &hasReviews,
		PendingActions: true,
	})
	if err != nil {
		t.Fatalf("ListPlaces() error = %v", err)
	}
	if result.Total != 1 || result.Results[0].Place.Name != first.Name {
		t.Fatalf("ListPlaces() = %#v, want first place only", result)
	}

	key := result.Results[0].PlaceKey
	updated, err := store.UpdatePlaceActions(ctx, 0, key, []models.ActionData{
		{"type": "call", "status": "pending", "reason": "high rating"},
	})
	if err != nil {
		t.Fatalf("UpdatePlaceActions() error = %v", err)
	}
	if len(updated.Actions) != 1 || updated.Actions[0]["type"] != "call" {
		t.Fatalf("updated.Actions = %#v", updated.Actions)
	}

	appended, err := store.AppendPlaceAction(ctx, 0, key, models.ActionData{"type": "email", "status": "queued"})
	if err != nil {
		t.Fatalf("AppendPlaceAction() error = %v", err)
	}
	if len(appended.Actions) != 2 {
		t.Fatalf("len(appended.Actions) = %d, want 2", len(appended.Actions))
	}

	missing, err := store.ListPlaces(ctx, PlaceListFilter{MissingActionType: "whatsapp"})
	if err != nil {
		t.Fatalf("ListPlaces(missing action) error = %v", err)
	}
	if missing.Total != 2 {
		t.Fatalf("missing.Total = %d, want 2", missing.Total)
	}
}

func readOneJSONLine[T any](t *testing.T, path string) T {
	t.Helper()

	values := readJSONLines[T](t, path)
	if len(values) != 1 {
		t.Fatalf("expected one JSONL record in %s, got %d", path, len(values))
	}
	return values[0]
}

func readJSONLines[T any](t *testing.T, path string) []T {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var values []T
	for scanner.Scan() {
		var value T
		if err := json.Unmarshal(scanner.Bytes(), &value); err != nil {
			t.Fatalf("decode JSONL record: %v", err)
		}
		values = append(values, value)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return values
}

func stringPtrForTest(value string) *string {
	return &value
}

package scraper

import (
	"strings"
	"testing"

	"mcp_server_scraper_googlemaps/internal/models"
)

func TestCleanTextRemovesGoogleMapsIconsAndControls(t *testing.T) {
	got := cleanText("\ue0c8 R. Mateus Leme,\n 1000\t - Centro")
	want := "R. Mateus Leme, 1000 - Centro"
	if got != want {
		t.Fatalf("cleanText() = %q, want %q", got, want)
	}
}

func TestCleanPhoneRemovesIconsAndKeepsPhonePunctuation(t *testing.T) {
	got := cleanPhone("\ue0b0(41) 3322-1441")
	want := "(41) 3322-1441"
	if got != want {
		t.Fatalf("cleanPhone() = %q, want %q", got, want)
	}
}

func TestSanitizePlacePageData(t *testing.T) {
	publishedAt := "\ue0be 2 semanas atras "
	rating := 5.0
	data := sanitizePlacePageData(placePageData{
		Name:     " Avenida Paulista \u200b",
		Address:  "\ue0c8 Rua Emiliano Perneta, 680",
		Phone:    "\ue0b0(41) 3322-1441",
		Website:  " https://example.com ",
		Category: "\ue0be Pizzaria",
		ImageURL: " https://img.example.com/photo.jpg ",
		Reviews: []models.ReviewData{
			{Author: " Cliente \u200b", Rating: &rating, PublishedAt: &publishedAt, Text: " Otimo atendimento\n "},
			{Author: " Cliente \u200b", Rating: &rating, PublishedAt: &publishedAt, Text: " Otimo atendimento\n "},
			{Author: "", Text: ""},
		},
	})

	if data.Name != "Avenida Paulista" {
		t.Fatalf("Name = %q", data.Name)
	}
	if data.Address != "Rua Emiliano Perneta, 680" {
		t.Fatalf("Address = %q", data.Address)
	}
	if data.Phone != "(41) 3322-1441" {
		t.Fatalf("Phone = %q", data.Phone)
	}
	if data.Website != "https://example.com" {
		t.Fatalf("Website = %q", data.Website)
	}
	if data.Category != "Pizzaria" {
		t.Fatalf("Category = %q", data.Category)
	}
	if data.ImageURL != "https://img.example.com/photo.jpg" {
		t.Fatalf("ImageURL = %q", data.ImageURL)
	}
	if len(data.Reviews) != 1 {
		t.Fatalf("len(Reviews) = %d, want 1: %#v", len(data.Reviews), data.Reviews)
	}
	if data.Reviews[0].Author != "Cliente" || data.Reviews[0].Text != "Otimo atendimento" {
		t.Fatalf("Reviews[0] = %#v, want cleaned review", data.Reviews[0])
	}
	if data.Reviews[0].PublishedAt == nil || *data.Reviews[0].PublishedAt != "2 semanas atras" {
		t.Fatalf("PublishedAt = %#v, want cleaned date", data.Reviews[0].PublishedAt)
	}
}

func TestCleanPhones(t *testing.T) {
	got := cleanPhones([]string{"\ue0b0(41) 3322-1441", " Whatsapp: (41) 99999-0000 "})
	want := []string{"(41) 3322-1441", "(41) 99999-0000"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestValidateInputRejectsTooManySearchQueries(t *testing.T) {
	input := models.Input{MaxPlacesPerQuery: 1}
	for i := 0; i < models.MaxSearchQueriesLimit+1; i++ {
		input.SearchQueries = append(input.SearchQueries, "query")
	}

	err := validateInput(input)
	if err == nil || !strings.Contains(err.Error(), "searchQueries") {
		t.Fatalf("validateInput() error = %v, want searchQueries limit error", err)
	}
}

func TestValidateInputRejectsTooManyPlaces(t *testing.T) {
	input := models.Input{
		SearchQueries:     []string{"pizzarias em Curitiba"},
		MaxPlacesPerQuery: models.MaxPlacesPerQueryLimit + 1,
	}

	err := validateInput(input)
	if err == nil || !strings.Contains(err.Error(), "maxPlacesPerQuery") {
		t.Fatalf("validateInput() error = %v, want maxPlacesPerQuery limit error", err)
	}
}

func TestValidateInputRejectsTooManyReviews(t *testing.T) {
	input := models.Input{
		SearchQueries:      []string{"pizzarias em Curitiba"},
		MaxPlacesPerQuery:  1,
		MaxReviewsPerPlace: models.MaxReviewsPerPlaceLimit + 1,
	}

	err := validateInput(input)
	if err == nil || !strings.Contains(err.Error(), "maxReviewsPerPlace") {
		t.Fatalf("validateInput() error = %v, want maxReviewsPerPlace limit error", err)
	}
}

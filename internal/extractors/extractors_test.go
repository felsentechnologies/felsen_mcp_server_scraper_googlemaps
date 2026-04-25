package extractors

import "testing"

func TestExtractEmails(t *testing.T) {
	html := `<a href="mailto:contato@empresa.com.br">Email</a><p>test@example.com</p><p>real@empresa.com.br</p>`
	got := ExtractEmails(html)
	want := []string{"contato@empresa.com.br", "real@empresa.com.br"}
	assertStrings(t, got, want)
}

func TestExtractPhones(t *testing.T) {
	html := `
		<head><script>{"id":"1234567890"}</script></head>
		<a href="tel:+556530527140">(65) 3052-7140</a>
		<a href="tel:17575368098">Fake</a>
		<p>(48) 99662-6260</p>
		<p>+44 20 7946-0958</p>`
	got := ExtractPhones(html)
	want := []string{"+556530527140", "(65) 3052-7140", "(48) 99662-6260", "+44 20 7946-0958"}
	assertStrings(t, got, want)
}

func TestExtractSocialLinks(t *testing.T) {
	html := `<a href="https://facebook.com/empresa">FB</a><a href="https://twitter.com/intent/tweet?x=y">Share</a>`
	got := ExtractSocialLinks(html)
	if got["facebook"] == nil || *got["facebook"] != "https://facebook.com/empresa" {
		t.Fatalf("facebook link mismatch: %#v", got["facebook"])
	}
	if got["twitter"] != nil {
		t.Fatalf("twitter intent link should be ignored")
	}
}

func TestFindContactPageURLs(t *testing.T) {
	html := `<a href="/contato">Contato</a><a href="https://other.test/contact">Outro</a><a href="contact-us">Contact</a>`
	got := FindContactPageURLs(html, "https://empresa.test/pages/")
	want := []string{"https://empresa.test/contato", "https://empresa.test/pages/contact-us"}
	assertStrings(t, got, want)
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mismatch at %d: got %#v want %#v", i, got, want)
		}
	}
}

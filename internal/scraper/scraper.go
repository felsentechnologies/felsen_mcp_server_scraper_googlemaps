package scraper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"mcp_server_scraper_googlemaps/internal/extractors"
	"mcp_server_scraper_googlemaps/internal/models"
)

type Scraper struct {
	logger *log.Logger
}

func New(logger *log.Logger) *Scraper {
	if logger == nil {
		logger = log.Default()
	}
	return &Scraper{logger: logger}
}

func (s *Scraper) ScrapeGoogleMaps(ctx context.Context, input models.Input) ([]models.PlaceData, error) {
	input = input.WithDefaults()
	if err := validateInput(input); err != nil {
		return nil, err
	}

	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"),
	)
	if execPath := browserExecPath(); execPath != "" {
		allocOpts = append(allocOpts, chromedp.ExecPath(execPath))
	}
	if input.ProxyConfiguration != nil && len(input.ProxyConfiguration.ProxyURLs) > 0 {
		allocOpts = append(allocOpts, chromedp.ProxyServer(input.ProxyConfiguration.ProxyURLs[0]))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()
	if err := chromedp.Run(browserCtx); err != nil {
		return nil, fmt.Errorf("start browser: %w", err)
	}

	placesByURL := make(map[string]models.PlaceData)
	for _, query := range input.SearchQueries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		s.logger.Printf("searching Google Maps: %q", query)
		placeURLs, err := s.collectPlaceURLs(browserCtx, query, input.Language, candidateLimit(input.MaxPlacesPerQuery))
		if err != nil {
			return nil, fmt.Errorf("collect place urls for %q: %w", query, err)
		}
		s.logger.Printf("found %d candidate place url(s) for %q", len(placeURLs), query)

		queryPlaces := 0
		for _, placeURL := range placeURLs {
			if queryPlaces >= input.MaxPlacesPerQuery {
				break
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			if _, exists := placesByURL[placeURL]; exists {
				continue
			}
			place, err := s.scrapePlace(browserCtx, query, placeURL)
			if err != nil {
				if isContextError(err) {
					return nil, err
				}
				s.logger.Printf("skipping %s: %v", placeURL, err)
				continue
			}
			placesByURL[placeURL] = place
			queryPlaces++
		}
	}

	places := make([]models.PlaceData, 0, len(placesByURL))
	for _, place := range placesByURL {
		places = append(places, place)
	}

	if (*input.ScrapeEmails || *input.ScrapePhones) && len(places) > 0 {
		s.enrichFromWebsites(ctx, places, *input.ScrapeEmails, *input.ScrapePhones)
	}

	for i := range places {
		places[i].Emails = unique(places[i].Emails)
		places[i].Phones = unique(places[i].Phones)
	}

	return places, nil
}

func (s *Scraper) collectPlaceURLs(ctx context.Context, query, language string, maxPlaces int) ([]string, error) {
	searchURL := fmt.Sprintf("https://www.google.com/maps/search/%s?hl=%s", url.QueryEscape(query), url.QueryEscape(language))
	tabCtx, cancelTab := chromedp.NewContext(ctx)
	defer cancelTab()
	timeoutCtx, cancelTimeout := context.WithTimeout(tabCtx, 2*time.Minute)
	defer cancelTimeout()

	var urls []string
	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(searchURL),
		chromedp.Sleep(2*time.Second),
		acceptConsent(),
		chromedp.WaitReady(`body`, chromedp.ByQuery),
		collectMapsPlaceURLs(maxPlaces, &urls),
	)
	if err != nil {
		return nil, err
	}
	return urls, nil
}

func (s *Scraper) scrapePlace(ctx context.Context, query, placeURL string) (models.PlaceData, error) {
	tabCtx, cancelTab := chromedp.NewContext(ctx)
	defer cancelTab()
	timeoutCtx, cancelTimeout := context.WithTimeout(tabCtx, 75*time.Second)
	defer cancelTimeout()

	var data placePageData
	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(placeURL),
		chromedp.Sleep(1500*time.Millisecond),
		acceptConsent(),
		chromedp.WaitReady(`body`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(placeExtractionScript, &data),
	)
	if err != nil {
		return models.PlaceData{}, err
	}
	data = sanitizePlacePageData(data)
	if strings.TrimSpace(data.Name) == "" {
		return models.PlaceData{}, errors.New("could not extract place name")
	}

	place := models.PlaceData{
		Query:         query,
		Name:          data.Name,
		Address:       stringPtr(data.Address),
		Phone:         stringPtr(data.Phone),
		Website:       stringPtr(data.Website),
		Rating:        floatPtr(data.Rating),
		ReviewsCount:  intPtr(data.ReviewsCount),
		Category:      stringPtr(data.Category),
		GoogleMapsURL: placeURL,
		ImageURL:      stringPtr(data.ImageURL),
		Emails:        []string{},
		Phones:        []string{},
		SocialLinks:   models.EmptySocialLinks(),
	}
	if place.Phone != nil {
		place.Phones = append(place.Phones, *place.Phone)
	}
	return place, nil
}

func (s *Scraper) enrichFromWebsites(ctx context.Context, places []models.PlaceData, scrapeEmails, scrapePhones bool) {
	client := &http.Client{
		Timeout: 20 * time.Second,
	}

	for i := range places {
		if places[i].Website == nil || *places[i].Website == "" {
			continue
		}
		mainURL := *places[i].Website
		html, err := fetchHTML(ctx, client, mainURL)
		if err != nil {
			s.logger.Printf("website fetch failed for %s: %v", places[i].Name, err)
			continue
		}
		applyContacts(&places[i], html, scrapeEmails, scrapePhones)

		visited := map[string]struct{}{mainURL: {}}
		for _, contactURL := range firstN(extractors.FindContactPageURLs(html, mainURL), 5) {
			if _, ok := visited[contactURL]; ok {
				continue
			}
			visited[contactURL] = struct{}{}
			subHTML, err := fetchHTML(ctx, client, contactURL)
			if err != nil {
				continue
			}
			applyContacts(&places[i], subHTML, scrapeEmails, scrapePhones)
		}
	}
}

func applyContacts(place *models.PlaceData, html string, scrapeEmails, scrapePhones bool) {
	if scrapeEmails {
		place.Emails = append(place.Emails, cleanValues(extractors.ExtractEmails(html))...)
	}
	if scrapePhones {
		place.Phones = append(place.Phones, cleanPhones(extractors.ExtractPhones(html))...)
	}
	for platform, link := range extractors.ExtractSocialLinks(html) {
		if link != nil && place.SocialLinks[platform] == nil {
			place.SocialLinks[platform] = link
		}
	}
}

func fetchHTML(ctx context.Context, client *http.Client, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("unexpected status %s", resp.Status)
	}
	if contentType != "" && !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "application/xhtml") {
		return "", fmt.Errorf("unsupported content type %s", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 3*1024*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

type placePageData struct {
	Name         string  `json:"name"`
	Address      string  `json:"address"`
	Phone        string  `json:"phone"`
	Website      string  `json:"website"`
	Rating       float64 `json:"rating"`
	ReviewsCount int     `json:"reviewsCount"`
	Category     string  `json:"category"`
	ImageURL     string  `json:"imageUrl"`
}

func acceptConsent() chromedp.Action {
	return chromedp.Evaluate(`(() => {
		const buttons = Array.from(document.querySelectorAll('button'));
		const button = buttons.find(b => /accept all|aceitar tudo|concordo/i.test(b.textContent || ''));
		if (button) button.click();
		return true;
	})()`, nil)
}

func collectMapsPlaceURLs(maxPlaces int, urls *[]string) chromedp.Action {
	return chromedp.Evaluate(fmt.Sprintf(`(async () => {
		const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
		const collect = found => {
			for (const link of Array.from(document.querySelectorAll('a[href*="/maps/place/"]'))) {
				if (link.href) found.add(link.href);
			}
		};
		const feed = document.querySelector('div[role="feed"]') || document.scrollingElement || document.body;
		const found = new Set();
		let noChangeCount = 0;
		let previousCount = 0;

		collect(found);
		for (let i = 0; i < 120 && noChangeCount < 8; i++) {
			const currentCount = found.size;
			if (currentCount >= %d) break;
			if (currentCount === previousCount) noChangeCount++;
			else {
				noChangeCount = 0;
				previousCount = currentCount;
			}
			feed.scrollBy(0, 10000);
			await sleep(1500);
			collect(found);
			const ended = Array.from(document.querySelectorAll('p > span > span'))
				.some(el => /end of|final da|nao encontrou|n.o encontrou/i.test(el.textContent || ''));
			if (ended) break;
		}
		return Array.from(found).slice(0, %d);
	})()`, maxPlaces, maxPlaces), urls, func(params *runtime.EvaluateParams) *runtime.EvaluateParams {
		return params.WithAwaitPromise(true).WithReturnByValue(true)
	})
}

const placeExtractionScript = `(() => {
	const text = el => (el && el.textContent ? el.textContent.trim() : '');
	const name = text(document.querySelector('h1'));
	const category = text(document.querySelector('button[jsaction*="pane.rating.category"]'));
	const infoItems = Array.from(document.querySelectorAll('[data-item-id]'));
	let address = '';
	let phone = '';
	let website = '';

	for (const item of infoItems) {
		const itemId = item.getAttribute('data-item-id') || '';
		const itemText = text(item);
		if (itemId === 'address' || itemId.startsWith('address')) address = itemText;
		if (itemId.startsWith('phone:') || itemId === 'phone') phone = itemText;
		if (itemId === 'authority') {
			const anchor = item.closest('a') || item.querySelector('a');
			website = anchor ? anchor.href : itemText;
		}
	}

	if (!address) {
		for (const btn of Array.from(document.querySelectorAll('button[aria-label]'))) {
			const label = btn.getAttribute('aria-label') || '';
			if (/endere|address/i.test(label)) {
				address = label.replace(/^(endere.{0,3}|address):?\s*/i, '').trim();
				break;
			}
		}
	}

	if (!phone) {
		for (const btn of Array.from(document.querySelectorAll('button[aria-label]'))) {
			const label = btn.getAttribute('aria-label') || '';
			if (/telefone|phone/i.test(label)) {
				phone = label.replace(/^(telefone|phone):?\s*/i, '').trim();
				break;
			}
		}
	}

	if (!website) {
		for (const link of Array.from(document.querySelectorAll('a[aria-label]'))) {
			const label = link.getAttribute('aria-label') || '';
			if (/site|website/i.test(label) && link.href && !link.href.includes('google.com/maps')) {
				website = link.href;
				break;
			}
		}
	}

	let rating = 0;
	const ratingEl = document.querySelector('div[role="img"][aria-label]');
	if (ratingEl) {
		const match = (ratingEl.getAttribute('aria-label') || '').match(/([\d,.]+)/);
		if (match) rating = Number.parseFloat(match[1].replace(',', '.')) || 0;
	}

	let reviewsCount = 0;
	const reviewBtn = document.querySelector('button[jsaction*="review"]');
	if (reviewBtn) {
		const match = (reviewBtn.textContent || '').match(/([\d.,]+)/);
		if (match) reviewsCount = Number.parseInt(match[1].replace(/[.,]/g, ''), 10) || 0;
	}

	const img = document.querySelector('img[src*="googleusercontent.com"]');
	return {
		name,
		address,
		phone,
		website,
		rating,
		reviewsCount,
		category,
		imageUrl: img ? img.src : ''
	};
})()`

func stringPtr(v string) *string {
	v = cleanText(v)
	if v == "" {
		return nil
	}
	return &v
}

func floatPtr(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

func intPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func firstN(values []string, n int) []string {
	if len(values) <= n {
		return values
	}
	return values[:n]
}

func sanitizePlacePageData(data placePageData) placePageData {
	data.Name = cleanText(data.Name)
	data.Address = cleanText(data.Address)
	data.Phone = cleanPhone(data.Phone)
	data.Website = cleanText(data.Website)
	data.Category = cleanText(data.Category)
	data.ImageURL = cleanText(data.ImageURL)
	return data
}

func cleanValues(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanText(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func cleanPhones(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanPhone(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func cleanText(value string) string {
	value = strings.Map(func(r rune) rune {
		if isGoogleMapsIconRune(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, value)
	value = whitespaceRegex.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func cleanPhone(value string) string {
	value = cleanText(value)
	filtered := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) || strings.ContainsRune("+()-. /", r) {
			return r
		}
		return -1
	}, value)
	filtered = whitespaceRegex.ReplaceAllString(filtered, " ")
	filtered = strings.TrimSpace(filtered)
	if match := phoneLikeRegex.FindString(filtered); match != "" {
		return strings.TrimSpace(match)
	}
	return filtered
}

func isGoogleMapsIconRune(r rune) bool {
	return r >= 0xE000 && r <= 0xF8FF
}

func candidateLimit(maxPlaces int) int {
	limit := maxPlaces * 3
	if limit < maxPlaces+10 {
		limit = maxPlaces + 10
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func validateInput(input models.Input) error {
	if len(input.SearchQueries) == 0 {
		return errors.New("at least one search query is required")
	}
	if len(input.SearchQueries) > models.MaxSearchQueriesLimit {
		return fmt.Errorf("searchQueries must contain at most %d items", models.MaxSearchQueriesLimit)
	}
	if input.MaxPlacesPerQuery < 1 {
		return errors.New("maxPlacesPerQuery must be greater than zero")
	}
	if input.MaxPlacesPerQuery > models.MaxPlacesPerQueryLimit {
		return fmt.Errorf("maxPlacesPerQuery must be less than or equal to %d", models.MaxPlacesPerQueryLimit)
	}
	return nil
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanText(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

var (
	whitespaceRegex = regexp.MustCompile(`\s+`)
	phoneLikeRegex  = regexp.MustCompile(`(?:\+?\d{1,3}[\s.-]?)?(?:\(\d{2,5}\)|\d{2,5})[\s.-]?\d{4,5}[\s.-]?\d{4}`)
)

func isContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func browserExecPath() string {
	for _, env := range []string{"CHROME_PATH", "CHROMIUM_PATH", "BROWSER_PATH"} {
		if value := strings.TrimSpace(os.Getenv(env)); value != "" {
			return value
		}
	}

	candidates := []string{
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

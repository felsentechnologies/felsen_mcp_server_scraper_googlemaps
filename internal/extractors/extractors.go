package extractors

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	emailRegex     = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	mailtoRegex    = regexp.MustCompile(`(?i)href=["']mailto:([^"'?]+)`)
	telRegex       = regexp.MustCompile(`(?i)href=["']tel:([^"']+)["']`)
	hrefRegex      = regexp.MustCompile(`(?i)href=["']([^"'#]+)["']`)
	brPhoneRegex   = regexp.MustCompile(`(?:\+55\s?)?\(\d{2}\)\s?\d{4,5}-\d{4}`)
	nanpPhoneRegex = regexp.MustCompile(`(?:\+?1[-. ]?)?\([2-9]\d{2}\)\s?[2-9]\d{2}[-.\s]\d{4}`)
	intlPhoneRegex = regexp.MustCompile(`\+\d{1,3}[\s.\-]\(?\d{1,5}\)?[\s.\-]\d{2,5}[\s.\-]\d{2,5}(?:[\s.\-]\d{1,5})?`)

	tagStripRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<head[\s\S]*?</head>`),
		regexp.MustCompile(`(?is)<script[\s\S]*?</script>`),
		regexp.MustCompile(`(?is)<style[\s\S]*?</style>`),
		regexp.MustCompile(`(?is)<svg[\s\S]*?</svg>`),
		regexp.MustCompile(`(?is)<noscript[\s\S]*?</noscript>`),
		regexp.MustCompile(`(?is)<template[\s\S]*?</template>`),
		regexp.MustCompile(`(?s)<!--[\s\S]*?-->`),
		regexp.MustCompile(`(?s)<[^>]+>`),
		regexp.MustCompile(`(?i)&[a-z]+;`),
	}

	socialPatterns = map[string]*regexp.Regexp{
		"facebook":  regexp.MustCompile(`(?i)https?://(?:www\.)?facebook\.com/[a-zA-Z0-9._-]+/?`),
		"instagram": regexp.MustCompile(`(?i)https?://(?:www\.)?instagram\.com/[a-zA-Z0-9._-]+/?`),
		"linkedin":  regexp.MustCompile(`(?i)https?://(?:www\.)?linkedin\.com/(?:company|in)/[a-zA-Z0-9._-]+/?`),
		"twitter":   regexp.MustCompile(`(?i)https?://(?:www\.)?(?:twitter|x)\.com/[a-zA-Z0-9._-]+/?`),
		"youtube":   regexp.MustCompile(`(?i)https?://(?:www\.)?youtube\.com/(?:channel|c|@|user/)[a-zA-Z0-9._-]+/?`),
	}
)

var validBrazilianDDDs = map[string]struct{}{
	"11": {}, "12": {}, "13": {}, "14": {}, "15": {}, "16": {}, "17": {}, "18": {}, "19": {},
	"21": {}, "22": {}, "24": {}, "27": {}, "28": {},
	"31": {}, "32": {}, "33": {}, "34": {}, "35": {}, "37": {}, "38": {},
	"41": {}, "42": {}, "43": {}, "44": {}, "45": {}, "46": {},
	"47": {}, "48": {}, "49": {},
	"51": {}, "53": {}, "54": {}, "55": {},
	"61": {}, "62": {}, "63": {}, "64": {}, "65": {}, "66": {}, "67": {}, "68": {}, "69": {},
	"71": {}, "73": {}, "74": {}, "75": {}, "77": {}, "79": {},
	"81": {}, "82": {}, "83": {}, "84": {}, "85": {}, "86": {}, "87": {}, "88": {}, "89": {},
	"91": {}, "92": {}, "93": {}, "94": {}, "95": {}, "96": {}, "97": {}, "98": {}, "99": {},
}

var ignoredEmailDomains = []string{
	"example.com", "sentry.io", "wixpress.com", "w3.org",
	"googleusercontent.com", "schema.org", "purl.org",
	"wordpress.org", "gravatar.com", "creativecommons.org",
	"cloudflare.com", "googleapis.com",
}

var ignoredEmailExtensions = []string{
	".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".css", ".js", ".woff",
}

var contactPaths = []string{
	"/contact", "/contacts", "/contact-us", "/contactus",
	"/contato", "/contatos", "/fale-conosco",
	"/about", "/about-us", "/aboutus",
	"/sobre", "/sobre-nos", "/quem-somos",
}

func ExtractEmails(html string) []string {
	all := make([]string, 0)
	for _, match := range mailtoRegex.FindAllStringSubmatch(html, -1) {
		all = append(all, strings.TrimSpace(match[1]))
	}
	all = append(all, emailRegex.FindAllString(html, -1)...)

	filtered := make([]string, 0, len(all))
	for _, email := range all {
		lower := strings.ToLower(email)
		if hasAnySuffix(lower, ignoredEmailExtensions) || containsIgnoredDomain(lower) {
			continue
		}
		filtered = append(filtered, email)
	}
	return unique(filtered)
}

func ExtractPhones(html string) []string {
	phones := make([]string, 0)
	for _, match := range telRegex.FindAllStringSubmatch(html, -1) {
		raw := regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(match[1]), " ")
		digits := onlyDigits(raw)
		if allSameDigit(digits) {
			continue
		}
		if strings.HasPrefix(raw, "+") {
			if len(digits) >= 7 && len(digits) <= 15 {
				phones = append(phones, raw)
			}
			continue
		}
		if isValidBrazilianPhone(digits) {
			phones = append(phones, raw)
		}
	}

	visible := html
	for _, re := range tagStripRegexes {
		visible = re.ReplaceAllString(visible, " ")
	}

	for _, match := range brPhoneRegex.FindAllString(visible, -1) {
		if isValidBrazilianPhone(onlyDigits(match)) {
			phones = append(phones, match)
		}
	}
	for _, match := range nanpPhoneRegex.FindAllString(visible, -1) {
		digits := onlyDigits(match)
		if (len(digits) == 10 || (len(digits) == 11 && digits[0] == '1')) && !allSameDigit(digits) {
			phones = append(phones, match)
		}
	}
	for _, match := range intlPhoneRegex.FindAllString(visible, -1) {
		digits := onlyDigits(match)
		if len(digits) >= 7 && len(digits) <= 15 && !allSameDigit(digits) {
			phones = append(phones, match)
		}
	}

	return unique(phones)
}

func ExtractSocialLinks(html string) map[string]*string {
	result := map[string]*string{
		"facebook": nil, "instagram": nil, "linkedin": nil, "twitter": nil, "youtube": nil,
	}
	hrefs := hrefRegex.FindAllStringSubmatch(html, -1)
	for platform, re := range socialPatterns {
		for _, match := range hrefs {
			href := match[1]
			if !re.MatchString(href) || strings.Contains(href, "/sharer") || strings.Contains(href, "intent/") {
				continue
			}
			v := href
			result[platform] = &v
			break
		}
	}
	return result
}

func FindContactPageURLs(html, baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil || base.Hostname() == "" {
		return nil
	}
	urls := make([]string, 0)
	for _, match := range hrefRegex.FindAllStringSubmatch(html, -1) {
		resolved, err := base.Parse(match[1])
		if err != nil || resolved.Hostname() != base.Hostname() {
			continue
		}
		path := strings.ToLower(resolved.Path)
		for _, cp := range contactPaths {
			if strings.Contains(path, cp) {
				urls = append(urls, resolved.String())
				break
			}
		}
	}
	return unique(urls)
}

func isValidBrazilianPhone(digits string) bool {
	var ddd, number string
	if strings.HasPrefix(digits, "55") && (len(digits) == 12 || len(digits) == 13) {
		ddd = digits[2:4]
		number = digits[4:]
	} else if len(digits) == 10 || len(digits) == 11 {
		ddd = digits[:2]
		number = digits[2:]
	} else {
		return false
	}
	if _, ok := validBrazilianDDDs[ddd]; !ok {
		return false
	}
	if len(number) == 9 {
		if number[0] != '9' {
			return false
		}
	} else if len(number) == 8 {
		if number[0] < '2' || number[0] > '8' {
			return false
		}
	} else {
		return false
	}
	return !allSameDigit(digits)
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func allSameDigit(s string) bool {
	if s == "" {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			return false
		}
	}
	return true
}

func hasAnySuffix(s string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

func containsIgnoredDomain(s string) bool {
	for _, domain := range ignoredEmailDomains {
		if strings.Contains(s, "@"+domain) || strings.Contains(s, domain) {
			return true
		}
	}
	return false
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok || value == "" {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

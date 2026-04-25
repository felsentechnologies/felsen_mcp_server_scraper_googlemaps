package models

type ProxyConfiguration struct {
	UseApifyProxy    bool     `json:"useApifyProxy,omitempty"`
	ApifyProxyGroups []string `json:"apifyProxyGroups,omitempty"`
	ProxyURLs        []string `json:"proxyUrls,omitempty"`
}

const (
	DefaultMaxPlacesPerQuery = 20
	MaxPlacesPerQueryLimit   = 500
	MaxSearchQueriesLimit    = 10
)

type Input struct {
	SearchQueries      []string            `json:"searchQueries"`
	MaxPlacesPerQuery  int                 `json:"maxPlacesPerQuery"`
	ScrapeEmails       *bool               `json:"scrapeEmails,omitempty"`
	ScrapePhones       *bool               `json:"scrapePhones,omitempty"`
	Language           string              `json:"language"`
	ProxyConfiguration *ProxyConfiguration `json:"proxyConfiguration,omitempty"`
}

type SocialLinks map[string]*string

type PlaceData struct {
	Query         string      `json:"query"`
	Name          string      `json:"name"`
	Address       *string     `json:"address"`
	Phone         *string     `json:"phone"`
	Website       *string     `json:"website"`
	Rating        *float64    `json:"rating"`
	ReviewsCount  *int        `json:"reviewsCount"`
	Category      *string     `json:"category"`
	GoogleMapsURL string      `json:"googleMapsUrl"`
	ImageURL      *string     `json:"imageUrl"`
	Emails        []string    `json:"emails"`
	Phones        []string    `json:"phones"`
	SocialLinks   SocialLinks `json:"socialLinks"`
}

func (i Input) WithDefaults() Input {
	if i.MaxPlacesPerQuery == 0 {
		i.MaxPlacesPerQuery = DefaultMaxPlacesPerQuery
	}
	if i.Language == "" {
		i.Language = "pt-BR"
	}
	if i.ScrapeEmails == nil {
		v := true
		i.ScrapeEmails = &v
	}
	if i.ScrapePhones == nil {
		v := true
		i.ScrapePhones = &v
	}
	return i
}

func EmptySocialLinks() SocialLinks {
	return SocialLinks{
		"facebook":  nil,
		"instagram": nil,
		"linkedin":  nil,
		"twitter":   nil,
		"youtube":   nil,
	}
}

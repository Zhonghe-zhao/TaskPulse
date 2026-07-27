package urlcheck

type Input struct {
	URLs []string `json:"urls"`
}

type ItemResult struct {
	URL        string `json:"url"`
	StatusCode int    `json:"status_code,omitempty"`
	FinalURL   string `json:"final_url,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

type Output struct {
	Total     int          `json:"total"`
	Succeeded int          `json:"succeeded"`
	Failed    int          `json:"failed"`
	Items     []ItemResult `json:"items"`
}

package llmanalysis

type Input struct {
	Subject string   `json:"subject"`
	Notes   []string `json:"notes"`
	Goal    string   `json:"goal"`
}

type Output struct {
	Subject string   `json:"subject"`
	Summary string   `json:"summary"`
	Plan    []string `json:"plan"`
	Model   string   `json:"model"`
}

type AnalysisRequest struct {
	Subject string
	Notes   []string
	Goal    string
}

type AnalysisResponse struct {
	Summary string
	Plan    []string
	Model   string
}

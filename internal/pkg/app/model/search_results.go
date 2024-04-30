package model

type SearchResults struct {
	Entries []SearchResultsEntry
}

type SearchResultsEntry struct {
	PageContent string  `json:"content"`
	Score       float32 `json:"score"`
	Filename    string  `json:"filename"`
}

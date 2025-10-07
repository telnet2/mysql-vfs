package models

// SearchResult represents a file found by search
type SearchResult struct {
	Name          string                 `json:"name"`
	DirectoryPath string                 `json:"directory_path"`
	Path          string                 `json:"path"`
	Size          int64                  `json:"size"`
	ContentType   string                 `json:"content_type"`
	Metadata      *string                `json:"-"` // Raw JSON string
	MetadataMap   map[string]interface{} `json:"metadata" gorm:"-"`
}

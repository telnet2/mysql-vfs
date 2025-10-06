package etc

import (
	"embed"
)

//go:embed schemas/*.schema.json
var FS embed.FS

// GetSchemaContent reads embedded schema file from schemas directory
func GetSchemaContent(filename string) ([]byte, error) {
	return FS.ReadFile("schemas/" + filename)
}

// ListSchemaFiles returns list of all embedded schema files
func ListSchemaFiles() ([]string, error) {
	entries, err := FS.ReadDir("schemas")
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

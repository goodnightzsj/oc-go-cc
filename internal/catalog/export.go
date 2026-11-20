package catalog

import (
	"encoding/json"
	"fmt"
)

// WriteFile marshals a catalog to JSON and writes it atomically to path.
func WriteFile(path string, catalog *Catalog) error {
	if catalog == nil {
		return fmt.Errorf("catalog is nil")
	}

	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}

	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("write catalog file: %w", err)
	}
	return nil
}

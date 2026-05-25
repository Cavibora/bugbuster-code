//go:build nochrome

package tools

import "fmt"

// fetchWithChromeImpl returns error when built without Chrome support
func fetchWithChromeImpl(pageURL, selector string) (string, error) {
	return "", fmt.Errorf("headless Chrome not available (built with -tags nochrome)")
}

package catalog

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

func normalizeInput(input CreateSubmissionInput) CreateSubmissionInput {
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Category = strings.TrimSpace(input.Category)
	input.SourceURL = strings.TrimSpace(input.SourceURL)
	input.OwnerContact = strings.TrimSpace(input.OwnerContact)
	return input
}

func validateInput(input CreateSubmissionInput) error {
	if len(input.Username) < 11 || len(input.Username) > 60 || !strings.HasSuffix(input.Username, "bot") {
		return fmt.Errorf("%w: username must be 11-60 characters and end in bot", ErrInvalidInput)
	}
	for _, symbol := range input.Username {
		if !((symbol >= 'a' && symbol <= 'z') || (symbol >= '0' && symbol <= '9') || symbol == '_') {
			return fmt.Errorf("%w: username can contain only lowercase latin letters, digits, and underscore", ErrInvalidInput)
		}
	}
	if len(input.Name) < 3 || len(input.Name) > 80 || len(input.Description) < 30 || len(input.Description) > 500 {
		return fmt.Errorf("%w: name or description length", ErrInvalidInput)
	}
	if !isCategory(input.Category) {
		return fmt.Errorf("%w: unknown category", ErrInvalidInput)
	}
	parsed, err := url.Parse(input.SourceURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("%w: source URL must be a valid https URL", ErrInvalidInput)
	}
	if len(input.OwnerContact) < 3 || len(input.OwnerContact) > 120 {
		return fmt.Errorf("%w: owner contact length", ErrInvalidInput)
	}
	return nil
}

func isCategory(value string) bool {
	for _, category := range Categories {
		if value == category {
			return true
		}
	}
	return false
}

func newID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		panic(fmt.Sprintf("generate random id: %v", err))
	}
	return hex.EncodeToString(bytes)
}

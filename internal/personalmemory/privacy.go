package personalmemory

import (
	"github.com/synergyai-os/Mindline/internal/contentguard"
)

func containsSecret(value string) bool {
	return contentguard.ContainsSecretLike(value)
}

func importedEvidenceContainsSecret(inputStrings ...string) bool {
	for _, value := range inputStrings {
		if contentguard.ContainsSecretLike(value) {
			return true
		}
	}
	return false
}

func containsUnsafeURL(value string) bool {
	return contentguard.ContainsNonPersistableURL(value)
}

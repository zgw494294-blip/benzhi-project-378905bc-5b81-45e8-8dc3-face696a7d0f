package domain

import "strings"

// ConsentAllowsPublication centralizes the policy check used by release
// summaries and keeps policy matching explicit and deterministic.
func ConsentAllowsPublication(state ConsentState, policy string) bool {
	if state == ConsentPending {
		return false
	}
	return state != ConsentRestricted || !strings.Contains(policy, "公开")
}

// RequiresConsentReview reports states that need a human decision.
func RequiresConsentReview(state ConsentState) bool {
	return state == ConsentPending || state == ConsentRestricted
}

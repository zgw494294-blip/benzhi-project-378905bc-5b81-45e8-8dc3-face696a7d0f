package quality

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// IsBlocking reports whether an issue prevents a quality pass.
func IsBlocking(issue Issue) bool { return issue.Severity == SeverityError }

// BlockingIssues selects a new slice so callers can safely annotate results.
func BlockingIssues(issues []Issue) []Issue {
	result := make([]Issue, 0, len(issues))
	for _, issue := range issues {
		if IsBlocking(issue) {
			result = append(result, issue)
		}
	}
	return result
}

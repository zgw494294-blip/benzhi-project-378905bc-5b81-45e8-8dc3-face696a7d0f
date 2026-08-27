package quality

import (
	"dialectarchive/internal/domain"
	"sort"
	"strings"
)

// SeveritySummary is the compact form used by batch audit cards.
type SeveritySummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

func (s SeveritySummary) Total() int { return s.Errors + s.Warnings }

func CountSeverity(issues []Issue) SeveritySummary {
	var summary SeveritySummary
	for _, issue := range issues {
		if strings.EqualFold(issue.Severity, "error") {
			summary.Errors++
		} else {
			summary.Warnings++
		}
	}
	return summary
}

// IssueIndex groups findings by object and code.  Grouping is deterministic,
// which keeps rendered reports and their tests stable across map iterations.
type IssueIndex struct {
	ByObject map[string][]Issue `json:"by_object"`
	ByCode   map[string]int     `json:"by_code"`
}

func IndexIssues(issues []Issue) IssueIndex {
	index := IssueIndex{ByObject: map[string][]Issue{}, ByCode: map[string]int{}}
	for _, issue := range issues {
		object := issue.ObjectID
		if object == "" {
			object = "batch"
		}
		index.ByObject[object] = append(index.ByObject[object], issue)
		index.ByCode[issue.Code]++
	}
	for object := range index.ByObject {
		sort.SliceStable(index.ByObject[object], func(i, j int) bool {
			if index.ByObject[object][i].Severity == index.ByObject[object][j].Severity {
				return index.ByObject[object][i].Code < index.ByObject[object][j].Code
			}
			return index.ByObject[object][i].Severity == "error"
		})
	}
	return index
}

// Merge combines checks while preserving issue order and recomputing the
// pass flag from severities.  Callers can safely pass zero-value results.
func Merge(results ...Result) Result {
	merged := Result{Passed: true, Issues: []Issue{}}
	for _, result := range results {
		merged.Issues = append(merged.Issues, result.Issues...)
		if !result.Passed {
			merged.Passed = false
		}
	}
	return merged
}

// BatchCheckInput is deliberately read-only: quality checks must never alter
// the aggregate that the application layer owns.
type BatchCheckInput struct {
	Batch       domain.CorpusBatch
	Segments    []domain.RecordingSegment
	Annotations map[string]domain.TranscriptAnnotation
}

type BatchCheckReport struct {
	Status          string          `json:"status"`
	Issues          []Issue         `json:"issues"`
	Severity        SeveritySummary `json:"severity"`
	PerObject       IssueIndex      `json:"per_object"`
	SegmentCount    int             `json:"segment_count"`
	AnnotationCount int             `json:"annotation_count"`
}

// EvaluateBatch executes the same deterministic rules as individual writes,
// but gives callers one complete report for a review or preflight screen.
func EvaluateBatch(input BatchCheckInput) BatchCheckReport {
	report := BatchCheckReport{Status: "passed", Issues: []Issue{}, SegmentCount: len(input.Segments), AnnotationCount: len(input.Annotations)}
	for _, segment := range input.Segments {
		segmentResult := Merge(CheckSegment(segment), CheckConsent(segment, input.Batch.ConsentPolicy))
		for i := range segmentResult.Issues {
			if segmentResult.Issues[i].ObjectID == "" {
				segmentResult.Issues[i].ObjectID = segment.ID
			}
		}
		report.Issues = append(report.Issues, segmentResult.Issues...)
		annotation, ok := input.Annotations[segment.ID]
		if !ok {
			report.Issues = append(report.Issues, Issue{Code: "MISSING_TRANSCRIPT", Message: "片段缺少当前转写", Severity: "error", ObjectID: segment.ID})
			continue
		}
		annotationResult := CheckAnnotation(annotation, segment)
		for i := range annotationResult.Issues {
			annotationResult.Issues[i].ObjectID = annotation.ID
		}
		report.Issues = append(report.Issues, annotationResult.Issues...)
	}
	timeline := CheckTimeline(input.Segments)
	report.Issues = append(report.Issues, timeline.Issues...)
	report.Severity = CountSeverity(report.Issues)
	report.PerObject = IndexIssues(report.Issues)
	if report.Severity.Errors > 0 {
		report.Status = "blocked"
	} else if report.Severity.Warnings > 0 {
		report.Status = "needs_fix"
	}
	return report
}

func SortedIssues(issues []Issue) []Issue {
	result := append([]Issue(nil), issues...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].ObjectID != result[j].ObjectID {
			return result[i].ObjectID < result[j].ObjectID
		}
		if result[i].Code != result[j].Code {
			return result[i].Code < result[j].Code
		}
		return result[i].Message < result[j].Message
	})
	return result
}

func HasCode(issues []Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func FilterSeverity(issues []Issue, severity string) []Issue {
	filtered := []Issue{}
	for _, issue := range issues {
		if strings.EqualFold(issue.Severity, severity) {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

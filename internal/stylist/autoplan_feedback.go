package stylist

import (
	"errors"
	"fmt"
)

type suggestRepairFeedback struct {
	Field        string `json:"field"`
	RejectedText string `json:"rejected_text"`
	MatchedText  string `json:"matched_text,omitempty"`
	Problem      string `json:"problem"`
}

type suggestSeriesFieldError struct {
	cause    error
	feedback suggestRepairFeedback
}

func (e *suggestSeriesFieldError) Error() string {
	if e.feedback.MatchedText != "" {
		return fmt.Sprintf("%s: «%s»", e.cause, quoteShort(e.feedback.MatchedText))
	}
	return e.cause.Error()
}

func (e *suggestSeriesFieldError) Unwrap() error { return e.cause }

func suggestFieldError(index int, field, label, value string, cause error) error {
	problem := cause.Error()
	matched := forbiddenRe.FindString(allowedRe.ReplaceAllString(value, ""))
	if matched != "" {
		problem = "Remove the matched person-identity attribute while preserving the intended styling. Automatic descriptions must not invent age, skin tone, ethnicity, body shape, facial features or hair colour. Return a complete corrected series."
	}
	return &suggestSeriesFieldError{
		cause:    fmt.Errorf("серия %d, %s: %w", index+1, label, cause),
		feedback: suggestRepairFeedback{Field: field, RejectedText: value, MatchedText: matched, Problem: problem},
	}
}

func suggestFailureFeedback(failures []error) *suggestRepairFeedback {
	if len(failures) == 0 {
		return nil
	}
	var fieldError *suggestSeriesFieldError
	if !errors.As(failures[0], &fieldError) {
		return nil
	}
	return &fieldError.feedback
}

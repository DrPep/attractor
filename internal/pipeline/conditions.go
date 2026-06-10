package pipeline

import (
	"fmt"
	"strings"

	"github.com/nigelpepper/attractor/internal/aerr"
)

// EvaluateCondition evaluates a condition expression against context and
// outcome. Supports "=", "!=", and "&&". An empty expression is always true.
func EvaluateCondition(expression string, ctx *PipelineContext, outcome, preferredLabel string) (bool, error) {
	expr := strings.TrimSpace(expression)
	if expr == "" {
		return true, nil
	}
	for _, clause := range strings.Split(expr, "&&") {
		ok, err := evalClause(strings.TrimSpace(clause), ctx, outcome, preferredLabel)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evalClause(clause string, ctx *PipelineContext, outcome, preferredLabel string) (bool, error) {
	if strings.Contains(clause, "!=") {
		parts := strings.SplitN(clause, "!=", 2)
		if len(parts) == 2 {
			l := resolveValue(strings.TrimSpace(parts[0]), ctx, outcome, preferredLabel)
			r := resolveValue(strings.TrimSpace(parts[1]), ctx, outcome, preferredLabel)
			return l != r, nil
		}
	}
	if strings.Contains(clause, "=") {
		parts := strings.SplitN(clause, "=", 2)
		if len(parts) == 2 {
			l := resolveValue(strings.TrimSpace(parts[0]), ctx, outcome, preferredLabel)
			r := resolveValue(strings.TrimSpace(parts[1]), ctx, outcome, preferredLabel)
			return l == r, nil
		}
	}
	return false, &aerr.ValidationError{Msg: fmt.Sprintf("Invalid condition clause: %s", clause)}
}

func resolveValue(token string, ctx *PipelineContext, outcome, preferredLabel string) string {
	token = strings.TrimSpace(token)
	if len(token) >= 2 {
		if (strings.HasPrefix(token, `"`) && strings.HasSuffix(token, `"`)) ||
			(strings.HasPrefix(token, "'") && strings.HasSuffix(token, "'")) {
			return token[1 : len(token)-1]
		}
	}
	switch token {
	case "outcome":
		return outcome
	case "preferred_label":
		return preferredLabel
	}
	if strings.HasPrefix(token, "context.") {
		return ctx.GetString(token[len("context."):])
	}
	return token
}

// ValidateCondition checks condition syntax, returning a list of error messages.
func ValidateCondition(expression string) []string {
	var errs []string
	expr := strings.TrimSpace(expression)
	if expr == "" {
		return errs
	}
	for _, clause := range strings.Split(expr, "&&") {
		clause = strings.TrimSpace(clause)
		var parts []string
		switch {
		case strings.Contains(clause, "!="):
			parts = strings.SplitN(clause, "!=", 2)
		case strings.Contains(clause, "="):
			parts = strings.SplitN(clause, "=", 2)
		default:
			errs = append(errs, fmt.Sprintf("Clause missing operator (= or !=): '%s'", clause))
			continue
		}
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			errs = append(errs, fmt.Sprintf("Invalid clause: '%s'", clause))
		}
	}
	return errs
}

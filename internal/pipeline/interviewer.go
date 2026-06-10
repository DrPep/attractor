package pipeline

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// QuestionType enumerates the kinds of human prompts.
type QuestionType string

const (
	QuestionSingleSelect QuestionType = "single_select"
	QuestionMultiSelect  QuestionType = "multi_select"
	QuestionFreeText     QuestionType = "free_text"
	QuestionConfirm      QuestionType = "confirm"
)

// Question is a prompt presented to a human.
type Question struct {
	Text          string
	Type          QuestionType
	Choices       []string
	DefaultChoice string
	Timeout       *time.Duration
}

// Answer is a human's response.
type Answer struct {
	Value  string
	Values []string
}

// Interviewer is the human-in-the-loop interface.
type Interviewer interface {
	Ask(q Question) (Answer, error)
}

// AutoApproveInterviewer always picks the first option; for automation/testing.
type AutoApproveInterviewer struct{}

// Ask returns the first choice (or "yes" for confirms).
func (AutoApproveInterviewer) Ask(q Question) (Answer, error) {
	if q.Type == QuestionConfirm {
		return Answer{Value: "yes"}, nil
	}
	if len(q.Choices) > 0 {
		return Answer{Value: q.Choices[0]}, nil
	}
	if q.DefaultChoice != "" {
		return Answer{Value: q.DefaultChoice}, nil
	}
	return Answer{}, nil
}

// ConsoleInterviewer prompts interactively on the terminal.
type ConsoleInterviewer struct{}

// Ask prompts the user and reads a response from stdin.
func (ConsoleInterviewer) Ask(q Question) (Answer, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\n%s\n", q.Text)

	switch q.Type {
	case QuestionConfirm:
		def := q.DefaultChoice
		if def == "" {
			def = "y"
		}
		fmt.Printf("[y/n] (default: %s): ", def)
		line, _ := reader.ReadString('\n')
		v := strings.ToLower(strings.TrimSpace(line))
		if v == "" {
			v = def
		}
		if strings.HasPrefix(v, "y") {
			return Answer{Value: "yes"}, nil
		}
		return Answer{Value: "no"}, nil

	case QuestionSingleSelect, QuestionMultiSelect:
		for i, c := range q.Choices {
			fmt.Printf("  %d. %s\n", i+1, c)
		}
		fmt.Print("Choose: ")
		line, _ := reader.ReadString('\n')
		resp := strings.TrimSpace(line)
		if resp == "" {
			resp = q.DefaultChoice
		}
		if i, err := strconv.Atoi(resp); err == nil && i > 0 && i <= len(q.Choices) {
			return Answer{Value: q.Choices[i-1]}, nil
		}
		for _, c := range q.Choices {
			if strings.HasPrefix(strings.ToLower(c), strings.ToLower(resp)) {
				return Answer{Value: c}, nil
			}
		}
		return Answer{Value: resp}, nil
	}

	fmt.Print("> ")
	line, _ := reader.ReadString('\n')
	return Answer{Value: strings.TrimSpace(line)}, nil
}

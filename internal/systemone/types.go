// Package systemone defines the native TypeSafe evaluation boundary.
package systemone

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Question struct {
	Type         string          `json:"type"`
	Instructions json.RawMessage `json:"instructions"`
	Criteria     json.RawMessage `json:"criteria,omitempty"`
}
type Request struct {
	Model     string              `json:"model"`
	State     json.RawMessage     `json:"state"`
	Questions map[string]Question `json:"questions"`
}
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
type Response struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
	Usage   Usage                      `json:"usage"`
}
type Evaluator interface {
	Evaluate(context.Context, *Request) (*Response, error)
}
type UpstreamError struct {
	Status     int
	RetryAfter string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("typesafe upstream returned status %d", e.Status)
}
func (e *UpstreamError) StatusCode() int { return e.Status }

var ErrUnsupported = errors.New("unsupported capability: TypeSafe supports only /v1/systemone evaluations")

func ParseRequest(body []byte) (*Request, error) {
	var req Request
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return nil, errors.New("unsupported System One request field")
		}
		return nil, errors.New("invalid System One request fields")
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, errors.New("invalid System One JSON body")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("model is required")
	}
	if !structured(req.State) {
		return nil, errors.New("state must be a string, object, or array")
	}
	if len(req.Questions) == 0 {
		return nil, errors.New("questions must be a non-empty map")
	}
	for _, q := range req.Questions {
		if err := validateQuestion(q); err != nil {
			return nil, err
		}
	}
	return &req, nil
}
func structured(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return false
	}
	return raw[0] == '"' || raw[0] == '{' || raw[0] == '['
}
func validateQuestion(q Question) error {
	if !structured(q.Instructions) {
		return errors.New("question instructions must be a string, object, or array")
	}
	switch q.Type {
	case "noul":
		if len(q.Criteria) == 0 {
			return nil
		}
		var values map[string]json.RawMessage
		if json.Unmarshal(q.Criteria, &values) != nil || values == nil {
			return errors.New("noul criteria must be an object")
		}
		for k, v := range values {
			if (k != "true" && k != "false") || !structured(v) {
				return errors.New("noul criteria requires structured true/false descriptions")
			}
		}
	case "choice":
		var values map[string]json.RawMessage
		if json.Unmarshal(q.Criteria, &values) != nil || len(values) == 0 || len(values) > 255 {
			return errors.New("choice criteria must contain 1 to 255 options")
		}
		for _, v := range values {
			if !structured(v) && string(v) != "null" {
				return errors.New("choice criteria descriptions must be structured or null")
			}
		}
	case "score":
		var values []json.RawMessage
		if json.Unmarshal(q.Criteria, &values) != nil || len(values) < 2 || len(values) > 10 {
			return errors.New("score criteria must contain 2 to 10 levels")
		}
		for _, v := range values {
			if !structured(v) {
				return errors.New("score criteria levels must be structured")
			}
		}
	default:
		return errors.New("unsupported question type; expected noul, choice, or score")
	}
	return nil
}

// ValidateResponse rejects incomplete successes without rewriting native answers.
func ValidateResponse(raw []byte, req *Request) (*Response, error) {
	var result Response
	var envelope struct {
		Usage *struct {
			Input  *int `json:"input_tokens"`
			Output *int `json:"output_tokens"`
		} `json:"usage"`
	}
	invalid := errors.New("typesafe returned an invalid evaluation response")
	if json.Unmarshal(raw, &result) != nil || json.Unmarshal(raw, &envelope) != nil {
		return nil, invalid
	}
	if result.Model == "" || envelope.Usage == nil {
		return nil, invalid
	}
	if envelope.Usage.Input == nil || envelope.Usage.Output == nil {
		return nil, invalid
	}
	if result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0 || len(result.Answers) != len(req.Questions) {
		return nil, invalid
	}
	for id, q := range req.Questions {
		var answer map[string]json.RawMessage
		if json.Unmarshal(result.Answers[id], &answer) != nil {
			return nil, invalid
		}
		var kind string
		if json.Unmarshal(answer["type"], &kind) != nil || kind != q.Type {
			return nil, invalid
		}
		switch kind {
		case "noul":
			if !numberInRange(answer["noul"], 0, 1) {
				return nil, invalid
			}
		case "choice", "score":
			if !numberInRange(answer["confidence"], 0, 1) {
				return nil, invalid
			}
			var probabilities map[string]float64
			if json.Unmarshal(answer["probabilities"], &probabilities) != nil || len(probabilities) == 0 {
				return nil, invalid
			}
			for _, v := range probabilities {
				if v < 0 || v > 1 {
					return nil, invalid
				}
			}
			if kind == "choice" {
				var choice string
				if json.Unmarshal(answer["choice"], &choice) != nil {
					return nil, invalid
				}
				if _, ok := probabilities[choice]; !ok {
					return nil, invalid
				}
			} else {
				var levels []json.RawMessage
				_ = json.Unmarshal(q.Criteria, &levels)
				if !numberInRange(answer["score"], 0, float64(len(levels)-1)) {
					return nil, invalid
				}
				var legend map[string]json.RawMessage
				if json.Unmarshal(answer["legend"], &legend) != nil || len(legend) != len(levels) {
					return nil, invalid
				}
			}
		}
	}
	return &result, nil
}
func numberInRange(raw json.RawMessage, min, max float64) bool {
	var n *float64
	return json.Unmarshal(raw, &n) == nil && n != nil && *n >= min && *n <= max
}

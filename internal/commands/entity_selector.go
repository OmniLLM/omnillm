package commands

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/manifoldco/promptui"
)

type entityChoice struct {
	id    string
	name  string
	alias string
}

// safeEntityText escapes server-supplied terminal controls without changing IDs.
func safeEntityText(value string) string {
	var result strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) {
			quoted := strconv.QuoteRune(r)
			result.WriteString(quoted[1 : len(quoted)-1])
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}

func (choice entityChoice) label() string {
	id := safeEntityText(choice.id)
	name := safeEntityText(strings.TrimSpace(choice.name))
	alias := safeEntityText(strings.TrimSpace(choice.alias))
	if name == "" && alias == "" {
		return id
	}
	if name == "" {
		return alias + " (" + id + ")"
	}
	if alias == "" {
		return name + " (" + id + ")"
	}
	return name + " [" + alias + "] (" + id + ")"
}

func (choice entityChoice) matches(input string) bool {
	return strings.Contains(strings.ToLower(choice.label()), strings.ToLower(strings.TrimSpace(input)))
}

func providerEntityChoices(data []byte) ([]entityChoice, error) {
	var providers []map[string]interface{}
	if err := json.Unmarshal(data, &providers); err != nil {
		return nil, err
	}
	choices := make([]entityChoice, 0, len(providers))
	for _, provider := range providers {
		id, _ := provider["id"].(string)
		if id == "" {
			continue
		}
		name, _ := provider["name"].(string)
		alias, _ := provider["alias"].(string)
		if strings.TrimSpace(alias) == "" {
			alias, _ = provider["subtitle"].(string)
		}
		choices = append(choices, entityChoice{id: id, name: name, alias: alias})
	}
	return choices, nil
}

func virtualModelEntityChoices(data []byte) ([]entityChoice, error) {
	var response map[string]interface{}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}
	items, _ := response["data"].([]interface{})
	choices := make([]entityChoice, 0, len(items))
	for _, item := range items {
		model, _ := item.(map[string]interface{})
		id, _ := model["virtual_model_id"].(string)
		if id == "" {
			continue
		}
		name, _ := model["name"].(string)
		choices = append(choices, entityChoice{id: id, name: name})
	}
	return choices, nil
}

func resolveEntityFromList(prompt string, choices []entityChoice) (string, error) {
	if len(choices) == 0 {
		return "", fmt.Errorf("no items available to select from")
	}
	if len(choices) == 1 {
		return choices[0].id, nil
	}
	labels := make([]string, len(choices))
	for i, choice := range choices {
		labels[i] = choice.label()
	}
	selectPrompt := promptui.Select{
		Label:             prompt,
		Items:             labels,
		Size:              len(labels),
		StartInSearchMode: true,
		Searcher: func(input string, index int) bool {
			return choices[index].matches(input)
		},
	}
	index, _, err := selectPrompt.Run()
	if err != nil {
		return "", err
	}
	return choices[index].id, nil
}

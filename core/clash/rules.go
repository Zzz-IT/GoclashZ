package clash

import (
	"encoding/json"
	"goclashz/core/utils"
	"os"
	"path/filepath"
)

type CustomRuleSet struct {
	CustomRules []string `json:"customRules"`
}

func GetCustomRules(id string) ([]string, error) {
	path := filepath.Join(utils.GetProfilesDir(), id+"_rules.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}, nil
	}
	var set CustomRuleSet
	json.Unmarshal(data, &set)
	return set.CustomRules, nil
}

func SaveCustomRules(id string, rules []string) error {
	path := filepath.Join(utils.GetProfilesDir(), id+"_rules.json")
	set := CustomRuleSet{CustomRules: rules}
	data, _ := json.MarshalIndent(set, "", "  ")
	return os.WriteFile(path, data, 0644)
}

// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"math"
	"sort"
)

// Report represents a complete lint report with scoring.
type Report struct {
	ProjectName  string          `json:"projectName"`
	Date         string          `json:"date"`
	OverallScore float64         `json:"overallScore"`
	Categories   []CategoryScore `json:"categories"`
	Violations   []Violation     `json:"-"`
	Summary      Summary         `json:"summary"`
}

// CategoryScore tracks the score for a lint category.
type CategoryScore struct {
	Name       string   `json:"name"`
	Score      float64  `json:"score"`      // 0-100
	Total      int      `json:"total"`      // elements checked
	Errors     int      `json:"errors"`     // error-severity violations
	Warnings   int      `json:"warnings"`   // warning-severity violations
	Infos      int      `json:"infos"`      // info-severity violations
	TopActions []string `json:"topActions"` // top recommendations
}

// categoryMapping maps rule IDs to category names.
var categoryMapping = map[string]string{
	// Naming
	"MPR001":  "Naming",
	"CONV001": "Naming",
	"CONV002": "Naming",
	"CONV003": "Naming",
	"CONV004": "Naming",
	"CONV005": "Naming",

	// Security
	"SEC001":  "Security",
	"SEC002":  "Security",
	"SEC003":  "Security",
	"SEC004":  "Security",
	"SEC005":  "Security",
	"SEC006":  "Security",
	"SEC007":  "Security",
	"SEC008":  "Security",
	"SEC009":  "Security",
	"CONV006": "Security",
	"CONV007": "Security",
	"CONV008": "Security",

	// Quality
	"MPR002":  "Quality",
	"MPR003":  "Quality",
	"MPR004":  "Quality",
	"MPR005":  "Quality",
	"MPR006":  "Quality",
	"QUAL001": "Quality",
	"QUAL002": "Quality",
	"QUAL003": "Quality",
	"QUAL004": "Quality",
	"QUAL005": "Quality",
	"CONV009": "Quality",
	"CONV012": "Quality",
	"CONV014": "Quality",
	"CONV015": "Quality",
	"CONV018": "Quality",

	// Architecture
	"ARCH001": "Architecture",
	"ARCH002": "Architecture",
	"ARCH003": "Architecture",
	"CONV010": "Architecture",

	// Performance
	"CONV011": "Performance",
	"CONV016": "Performance",
	"CONV017": "Performance",

	// Design
	"DESIGN001": "Design",
	"MPR007":    "Design",
	"CONV013":   "Design",
}

// categoryWeight defines the weight for each category in overall score.
var categoryWeight = map[string]float64{
	"Security":     0.25,
	"Quality":      0.20,
	"Architecture": 0.15,
	"Performance":  0.15,
	"Naming":       0.10,
	"Design":       0.10,
	"Other":        0.05,
}

// BuildReport creates a Report from a list of violations.
func BuildReport(projectName, date string, violations []Violation) *Report {
	report := &Report{
		ProjectName: projectName,
		Date:        date,
		Violations:  violations,
		Summary:     Summarize(violations),
	}

	// Group violations by category
	catViolations := make(map[string][]Violation)
	for _, v := range violations {
		cat := resolveCategory(v.RuleID)
		catViolations[cat] = append(catViolations[cat], v)
	}

	// Build category scores
	var categories []CategoryScore
	allCats := []string{"Security", "Quality", "Architecture", "Performance", "Naming", "Design"}
	for _, catName := range allCats {
		vols := catViolations[catName]
		cs := buildCategoryScore(catName, vols)
		categories = append(categories, cs)
	}

	// Add "Other" category for unmapped rules
	if otherVols := catViolations["Other"]; len(otherVols) > 0 {
		cs := buildCategoryScore("Other", otherVols)
		categories = append(categories, cs)
	}

	report.Categories = categories

	// Compute overall score as weighted average
	var totalWeight float64
	var weightedScore float64
	for _, cs := range categories {
		w := categoryWeight[cs.Name]
		if w == 0 {
			w = 0.05
		}
		totalWeight += w
		weightedScore += cs.Score * w
	}
	if totalWeight > 0 {
		report.OverallScore = math.Round(weightedScore/totalWeight*10) / 10
	} else {
		report.OverallScore = 100
	}

	return report
}

func resolveCategory(ruleID string) string {
	if cat, ok := categoryMapping[ruleID]; ok {
		return cat
	}
	return "Other"
}

func buildCategoryScore(name string, violations []Violation) CategoryScore {
	cs := CategoryScore{Name: name}

	for _, v := range violations {
		switch v.Severity {
		case SeverityError:
			cs.Errors++
		case SeverityWarning:
			cs.Warnings++
		case SeverityInfo:
			cs.Infos++
		}
	}

	cs.Total = cs.Errors + cs.Warnings + cs.Infos

	// Scoring: Error=-10, Warning=-3, Info=-1
	penalty := float64(cs.Errors)*10 + float64(cs.Warnings)*3 + float64(cs.Infos)*1
	cs.Score = math.Max(0, 100-penalty)

	// Build top actions from most frequent violation messages (deduplicated by rule)
	ruleMessages := make(map[string]string)
	ruleCounts := make(map[string]int)
	for _, v := range violations {
		ruleCounts[v.RuleID]++
		if _, ok := ruleMessages[v.RuleID]; !ok && v.Suggestion != "" {
			ruleMessages[v.RuleID] = v.Suggestion
		}
	}

	// Sort by count descending
	type ruleCount struct {
		ruleID string
		count  int
	}
	var sorted []ruleCount
	for id, c := range ruleCounts {
		sorted = append(sorted, ruleCount{id, c})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	for i, rc := range sorted {
		if i >= 5 {
			break
		}
		if msg, ok := ruleMessages[rc.ruleID]; ok {
			cs.TopActions = append(cs.TopActions, msg)
		}
	}

	return cs
}

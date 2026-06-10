package agent

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nigelpepper/attractor/internal/agent/tools"
)

// Skill is a reusable bundle of system-prompt additions and tool-set changes.
type Skill struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	SystemPrompt string   `yaml:"system_prompt"`
	ToolsInclude []string `yaml:"tools_include"`
	ToolsExclude []string `yaml:"tools_exclude"`
}

// ComposedSkill is the flattened result of merging multiple skills.
type ComposedSkill struct {
	SystemPrompt string
	ToolsExclude []string
	ExtraTools   []tools.Tool
}

// SkillRegistry loads and composes skills.
type SkillRegistry struct {
	skills      map[string]Skill
	customTools map[string][]tools.Tool
	order       []string
}

// NewSkillRegistry returns an empty registry.
func NewSkillRegistry() *SkillRegistry {
	return &SkillRegistry{skills: map[string]Skill{}, customTools: map[string][]tools.Tool{}}
}

// Register adds a skill programmatically, optionally with custom tools.
func (r *SkillRegistry) Register(s Skill, customTools []tools.Tool) {
	if _, exists := r.skills[s.Name]; !exists {
		r.order = append(r.order, s.Name)
	}
	r.skills[s.Name] = s
	if len(customTools) > 0 {
		r.customTools[s.Name] = customTools
	}
}

// Get returns a skill by name, or (zero, false).
func (r *SkillRegistry) Get(name string) (Skill, bool) { s, ok := r.skills[name]; return s, ok }

// ListSkills returns all skills in registration order.
func (r *SkillRegistry) ListSkills() []Skill {
	out := make([]Skill, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.skills[n])
	}
	return out
}

// LoadDir loads skills from a directory (.yaml/.yml files). Python (.py) skill
// modules are not supported in the Go build and are skipped with a warning.
func (r *SkillRegistry) LoadDir(path string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		log.Printf("Skill directory does not exist: %s", path)
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		full := filepath.Join(path, name)
		switch {
		case strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml"):
			r.loadYAML(full)
		case strings.HasSuffix(name, ".py") && !strings.HasPrefix(name, "_"):
			log.Printf("Skipping Python skill module (unsupported in Go build): %s", full)
		}
	}
}

func splitFrontmatter(text string) (string, string, bool) {
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return "", text, false
	}
	lines := strings.SplitAfter(text, "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[1:i], ""), strings.Join(lines[i+1:], ""), true
		}
	}
	return "", text, false
}

func (r *SkillRegistry) loadYAML(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Failed to load skill from %s: %v", path, err)
		return
	}
	text := string(data)
	frontmatter, body, hasFM := splitFrontmatter(text)

	var skill Skill
	if hasFM {
		if err := yaml.Unmarshal([]byte(frontmatter), &skill); err != nil {
			log.Printf("Failed to load skill from %s: %v", path, err)
			return
		}
		if skill.Name == "" {
			skill.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		if strings.TrimSpace(body) != "" {
			skill.SystemPrompt = strings.TrimSpace(body)
		}
	} else {
		if err := yaml.Unmarshal(data, &skill); err != nil {
			log.Printf("Failed to load skill from %s: %v", path, err)
			return
		}
	}
	if skill.Name == "" {
		log.Printf("Skill file %s has no name", path)
		return
	}
	r.Register(skill, nil)
}

// Compose merges named skills: prompts concatenated, excludes unioned, custom
// tools collected. Unknown names are logged and skipped.
func (r *SkillRegistry) Compose(names []string) ComposedSkill {
	var promptParts []string
	var excludes []string
	seenExclude := map[string]bool{}
	var extraTools []tools.Tool

	for _, name := range names {
		skill, ok := r.skills[name]
		if !ok {
			log.Printf("Unknown skill referenced: %q", name)
			continue
		}
		if skill.SystemPrompt != "" {
			promptParts = append(promptParts, strings.TrimSpace(skill.SystemPrompt))
		}
		for _, ex := range skill.ToolsExclude {
			if !seenExclude[ex] {
				seenExclude[ex] = true
				excludes = append(excludes, ex)
			}
		}
		extraTools = append(extraTools, r.customTools[name]...)
	}

	return ComposedSkill{
		SystemPrompt: strings.Join(promptParts, "\n\n"),
		ToolsExclude: excludes,
		ExtraTools:   extraTools,
	}
}

// BuildToolRegistry builds a registry from defaults, applying skill changes.
func (r *SkillRegistry) BuildToolRegistry(composed ComposedSkill) *tools.ToolRegistry {
	registry := tools.CreateDefaultRegistry()
	for _, name := range composed.ToolsExclude {
		registry.Remove(name)
	}
	for _, t := range composed.ExtraTools {
		registry.Register(t)
	}
	return registry
}

package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Skill struct {
	Name                string `json:"name"`
	Description         string `json:"description"`
	DisableModelInvocation bool  `json:"disable_model_invocation"`
	Content             string `json:"content"`
	FilePath            string `json:"file_path"`
}

func skillDirs() []string {
	wd, _ := os.Getwd()
	homeDir, _ := os.UserHomeDir()
	return []string{
		filepath.Join(wd, ".openaide", "skills"),
		filepath.Join(homeDir, ".openaide", "skills"),
	}
}

func LoadSkills() []Skill {
	var skills []Skill
	seen := make(map[string]bool)

	for _, dir := range skillDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
			content, err := os.ReadFile(skillPath)
			if err != nil {
				continue
			}
			if seen[entry.Name()] {
				continue
			}
			seen[entry.Name()] = true

			skill := parseSkillMD(string(content))
			skill.Name = entry.Name()
			skill.FilePath = skillPath
			skills = append(skills, skill)
		}
	}
	return skills
}

func LoadSkill(name string) *Skill {
	for _, dir := range skillDirs() {
		skillPath := filepath.Join(dir, name, "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}
		skill := parseSkillMD(string(content))
		skill.Name = name
		skill.FilePath = skillPath
		return &skill
	}
	return nil
}

func parseSkillMD(content string) Skill {
	skill := Skill{}

	if strings.HasPrefix(content, "---") {
		end := strings.Index(content[3:], "---")
		if end >= 0 {
			frontmatter := content[3 : end+3]
			skill.Content = strings.TrimSpace(content[end+6:])

			for _, line := range strings.Split(frontmatter, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || line == "---" {
					continue
				}
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					continue
				}
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				switch key {
				case "name":
					skill.Name = val
				case "description":
					skill.Description = val
				case "disable-model-invocation":
					skill.DisableModelInvocation = val == "true"
				}
			}
		}
	} else {
		skill.Content = strings.TrimSpace(content)
	}

	return skill
}

func CreateSkill(name, description, content string) error {
	wd, _ := os.Getwd()
	skillDir := filepath.Join(wd, ".openaide", "skills", name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("name: %s\n", name))
	sb.WriteString(fmt.Sprintf("description: %s\n", description))
	sb.WriteString("disable-model-invocation: false\n")
	sb.WriteString("---\n\n")
	sb.WriteString(content)

	return os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(sb.String()), 0644)
}

func RenderSkillList(skills []Skill) {
	if len(skills) == 0 {
		RenderInfoLine("No skills found. Create one with /skill create <name>")
		return
	}

	fmt.Println()
	fmt.Printf("  %s\n\n", R.Bold.Render("Skills"))

	for _, skill := range skills {
		desc := skill.Description
		if desc == "" {
			desc = "No description"
		}
		autoLabel := ""
		if skill.DisableModelInvocation {
			autoLabel = R.Warning.Render(" [manual]")
		}
		fmt.Printf("  %s %s%s\n",
			R.Accent.Render(skill.Name),
			R.Dim.Render(truncateStr(desc, 60)),
			autoLabel)
	}

	fmt.Printf("\n  %s\n", R.Dim.Render("Use /skill <name> to run a skill"))
	fmt.Printf("  %s\n", R.Dim.Render("Use /skill create <name> to create a new skill"))
}

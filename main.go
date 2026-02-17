// bmad2vibe converts BMAD Method artifacts into Mistral Vibe format.
//
// Mapping:
//
//	BMAD Agent       → Vibe Agent (.toml) + Vibe Prompt (.md)
//	BMAD Command     → Vibe Agent (.toml) for workflow commands, or inlined in prompt
//	BMAD Workflow    → Vibe Skill (SKILL.md) + inlined steps, referenced from agent prompts
//	BMAD Task/Tool   → Vibe Skill (SKILL.md), user-invocable
//
// Usage:
//
//	bmad2vibe [flags]
//	  -vibe-home    string  Vibe home directory (default ~/.vibe)
//	  -modules      string  Comma-separated modules to convert (default "bmm,cis,bmgd")
//	  -dry-run              Show what would be done
//	  -verbose              Verbose output
//	  -cleanup              Remove temp repos after conversion (default true)
//	  -bundles-dir  string  Use local bmad-bundles instead of cloning
//	  -method-dir   string  Use local BMAD-METHOD instead of cloning
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	bmadBundlesRepo = "https://github.com/bmad-code-org/bmad-bundles.git"
	bmadMethodRepo  = "https://github.com/bmad-code-org/BMAD-METHOD.git"
)

// --- Safety and tools mapping ---

var agentSafetyMap = map[string]string{
	// BMM agents
	"analyst": "safe", "architect": "safe", "pm": "safe",
	"sm": "safe", "tea": "safe", "tech-writer": "safe",
	"ux-designer": "safe", "dev": "destructive",
	"quick-flow-solo-dev": "destructive",
	// BMGD agents
	"game-dev": "destructive", "game-solo-dev": "destructive",
	"game-architect": "safe", "game-designer": "safe",
	"game-scrum-master": "safe", "game-qa": "safe",
	// CIS agents
	"brainstorming-coach": "safe", "creative-problem-solver": "safe",
	"design-thinking-coach": "safe", "innovation-strategist": "safe",
	"presentation-master": "safe", "storyteller": "safe",
	// BMB agents
	"bmad-builder": "destructive", "agent-builder": "destructive",
	"module-builder": "destructive", "workflow-builder": "destructive",
}

var safetyToolsMap = map[string][]string{
	"safe":        {"read_file", "grep", "list_dir", "ask_user_question"},
	"neutral":     {"read_file", "grep", "list_dir", "write_file", "search_replace", "ask_user_question"},
	"destructive": {"read_file", "grep", "list_dir", "write_file", "search_replace", "bash", "ask_user_question", "task"},
}

// --- Types ---

type config struct {
	vibeHome string
	modules  []string
	dryRun   bool
	verbose  bool
	cleanup  bool
	tmpDir   string
}

type conversionReport struct {
	agents   []string
	prompts  []string
	skills   []string
	warnings []string
	errors   []string
}

func (r *conversionReport) warn(msg string) { r.warnings = append(r.warnings, msg) }
func (r *conversionReport) err(msg string)  { r.errors = append(r.errors, msg) }

type agentMeta struct {
	Slug        string
	Name        string // persona name (e.g. "Barry")
	Title       string // role title (e.g. "Quick Flow Solo Dev")
	Icon        string
	Description string
}

// --- Main ---

func main() {
	var (
		vibeHome   = flag.String("vibe-home", "", "Vibe home directory (default ~/.vibe)")
		modules    = flag.String("modules", "bmm,cis,bmgd", "Comma-separated modules to convert")
		dryRun     = flag.Bool("dry-run", false, "Show what would be done without writing files")
		verbose    = flag.Bool("verbose", false, "Verbose output")
		cleanup    = flag.Bool("cleanup", true, "Remove temp cloned repos after conversion")
		bundlesDir = flag.String("bundles-dir", "", "Use local bmad-bundles dir instead of cloning")
		methodDir  = flag.String("method-dir", "", "Use local BMAD-METHOD dir instead of cloning")
	)
	flag.Parse()

	if *vibeHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("cannot determine home directory: %v", err)
		}
		*vibeHome = filepath.Join(home, ".vibe")
	}

	tmpDir, err := os.MkdirTemp("", "bmad2vibe-*")
	if err != nil {
		log.Fatalf("cannot create temp directory: %v", err)
	}
	if *cleanup {
		defer os.RemoveAll(tmpDir)
	} else {
		fmt.Printf("📁 Temp directory: %s\n", tmpDir)
	}

	cfg := &config{
		vibeHome: *vibeHome,
		modules:  splitTrim(*modules, ","),
		dryRun:   *dryRun,
		verbose:  *verbose,
		cleanup:  *cleanup,
		tmpDir:   tmpDir,
	}

	report := &conversionReport{}

	fmt.Println("🚀 bmad2vibe — BMAD Method → Mistral Vibe converter")
	fmt.Printf("   Target: %s\n", cfg.vibeHome)
	fmt.Printf("   Modules: %v\n", cfg.modules)
	if cfg.dryRun {
		fmt.Println("   ⚠️  DRY RUN — no files will be written")
	}
	fmt.Println()

	// Step 1: Get sources
	bDir, mDir := resolveSources(cfg, *bundlesDir, *methodDir)

	// Step 2: Create target dirs
	ensureDirs(cfg, "agents", "prompts", "skills")

	// Phase 1: Agents (bundles XML → TOML + prompt)
	fmt.Println("📋 Phase 1: Converting agents...")
	for _, mod := range cfg.modules {
		convertAgents(cfg, mod, bDir, report)
	}

	// Phase 2: Workflows → skills
	fmt.Println("\n⚙️  Phase 2: Converting workflows → skills...")
	for _, mod := range cfg.modules {
		convertWorkflows(cfg, mod, mDir, report)
	}

	// Phase 3: Tasks/tools → skills
	fmt.Println("\n🔧 Phase 3: Converting tasks/tools → skills...")
	for _, mod := range cfg.modules {
		convertTasks(cfg, mod, mDir, report)
	}

	// Phase 4: Workflow shortcut agents
	fmt.Println("\n🎯 Phase 4: Generating workflow shortcut agents...")
	for _, mod := range cfg.modules {
		generateWorkflowAgents(cfg, mod, mDir, report)
	}

	// Phase 5: Copy supporting data
	fmt.Println("\n📄 Phase 5: Copying supporting data...")
	for _, mod := range cfg.modules {
		copyModuleData(cfg, mod, mDir, report)
	}

	// Phase 6: AGENTS.md
	fmt.Println("\n📝 Phase 6: Generating AGENTS.md...")
	generateAgentsMD(cfg, report)

	// Phase 7: Validate
	fmt.Println("\n🔍 Phase 7: Validating...")
	validate(cfg, report)

	printReport(cfg, report)
}

// --- Source resolution ---

func resolveSources(cfg *config, bundlesFlag, methodFlag string) (string, string) {
	bDir := bundlesFlag
	mDir := methodFlag

	if bDir == "" {
		bDir = filepath.Join(cfg.tmpDir, "bmad-bundles")
		if err := cloneRepo(bmadBundlesRepo, bDir, cfg.verbose); err != nil {
			log.Fatalf("failed to clone bmad-bundles: %v", err)
		}
	} else {
		fmt.Printf("   📂 Using local bundles: %s\n", bDir)
	}

	if mDir == "" {
		mDir = filepath.Join(cfg.tmpDir, "BMAD-METHOD")
		if err := cloneRepo(bmadMethodRepo, mDir, cfg.verbose); err != nil {
			log.Fatalf("failed to clone BMAD-METHOD: %v", err)
		}
	} else {
		fmt.Printf("   📂 Using local method: %s\n", mDir)
	}
	return bDir, mDir
}

func cloneRepo(url, dest string, verbose bool) error {
	fmt.Printf("   📥 Cloning %s...\n", url)
	cmd := exec.Command("git", "clone", "--depth", "1", url, dest)
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
	}
	return cmd.Run()
}

func ensureDirs(cfg *config, subdirs ...string) {
	if cfg.dryRun {
		return
	}
	for _, s := range subdirs {
		os.MkdirAll(filepath.Join(cfg.vibeHome, s), 0o755)
	}
}

// --- Phase 1: Agent conversion (XML bundles → TOML + prompt) ---

func convertAgents(cfg *config, module, bundlesDir string, report *conversionReport) {
	agentsDir := filepath.Join(bundlesDir, module, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		report.warn(fmt.Sprintf("no agents dir for module %q in bundles", module))
		return
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".xml") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".xml")
		xmlPath := filepath.Join(agentsDir, e.Name())

		raw, err := os.ReadFile(xmlPath)
		if err != nil {
			report.err(fmt.Sprintf("agent %s/%s: read: %v", module, slug, err))
			continue
		}
		rawStr := string(raw)

		meta := extractAgentMeta(slug, rawStr)
		vibeSlug := fmt.Sprintf("bmad-%s-%s", module, slug)
		safety := safetyForAgent(slug)

		toml := buildAgentTOML(vibeSlug, module, meta, safety)
		tomlPath := filepath.Join(cfg.vibeHome, "agents", vibeSlug+".toml")

		prompt := buildAgentPrompt(module, slug, meta, rawStr)
		promptPath := filepath.Join(cfg.vibeHome, "prompts", vibeSlug+".md")

		if cfg.verbose {
			fmt.Printf("   ✅ %s/%s → agent + prompt\n", module, slug)
		}

		writeFile(cfg, tomlPath, toml, report)
		writeFile(cfg, promptPath, prompt, report)
		report.agents = append(report.agents, vibeSlug)
		report.prompts = append(report.prompts, vibeSlug)
	}
}

func buildAgentTOML(vibeSlug, module string, meta agentMeta, safety string) string {
	tools := safetyToolsMap[safety]
	displayName := fmt.Sprintf("BMAD %s %s", strings.ToUpper(module), meta.Title)
	if meta.Name != "" && meta.Name != meta.Title {
		displayName += fmt.Sprintf(" (%s)", meta.Name)
	}
	desc := meta.Description
	if desc == "" {
		desc = fmt.Sprintf("BMAD %s agent: %s", strings.ToUpper(module), meta.Title)
	}

	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }

	w("# Auto-generated by bmad2vibe\n")
	w("# BMAD Agent: %s\n", vibeSlug)
	w("# Source module: %s | Persona: %s %s\n\n", module, meta.Icon, meta.Name)
	w("display_name = %q\n", displayName)
	w("description = %q\n", desc)
	w("safety = %q\n", safety)
	w("auto_approve = %v\n", safety == "safe")
	w("system_prompt_id = %q\n", vibeSlug)
	w("\nenabled_tools = [%s]\n", joinQuoted(tools))

	return b.String()
}

func buildAgentPrompt(module, slug string, meta agentMeta, rawXML string) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }

	w("# %s %s", meta.Icon, meta.Title)
	if meta.Name != "" {
		w(" (%s)", meta.Name)
	}
	w("\n\n")
	w("> Module: %s | Agent: %s | Generated by bmad2vibe\n\n", strings.ToUpper(module), slug)

	// Vibe adaptation layer — critical for correct execution
	w("## Vibe Runtime Adaptation\n\n")
	w("You are running inside **Mistral Vibe** CLI, NOT Claude Code/Cursor/Windsurf.\n")
	w("Apply these substitutions when following BMAD instructions:\n\n")
	w("| BMAD reference | Vibe equivalent |\n")
	w("|---|---|\n")
	w("| `{project-root}` | Current working directory |\n")
	w("| `{output_folder}` | `_bmad-output/` |\n")
	w("| `{planning_artifacts}` | `_bmad-output/planning-artifacts/` |\n")
	w("| `{implementation_artifacts}` | `_bmad-output/implementation-artifacts/` |\n")
	w("| Slash commands (`/bmad-...`) | Execute the workflow instructions inline |\n")
	w("| `ask_user_question` | Vibe interactive question tool |\n")
	w("| `workflow.xml` engine | Follow workflow steps sequentially |\n")
	w("| `task` tool (subagent) | Vibe `task` tool for delegation |\n\n")

	w("When a menu item references a workflow, read its SKILL.md from\n")
	w("`~/.vibe/skills/bmad-%s-<workflow-name>/SKILL.md` and execute it.\n\n", module)

	// Full BMAD agent — LLMs handle XML natively
	w("## Full Agent Definition\n\n")
	w("Follow the agent specification below exactly, adapting tool calls to Vibe.\n\n")
	w("```xml\n%s\n```\n", strings.TrimSpace(rawXML))

	return b.String()
}

// --- Phase 2: Workflow → skill conversion ---

func convertWorkflows(cfg *config, module, methodDir string, report *conversionReport) {
	workflowsDir := filepath.Join(methodDir, "src", "modules", module, "workflows")
	if !dirExists(workflowsDir) {
		report.warn(fmt.Sprintf("no workflows dir for module %q", module))
		return
	}

	filepath.Walk(workflowsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		if !strings.HasPrefix(name, "workflow") || !strings.HasSuffix(name, ".md") {
			return nil
		}

		rel, _ := filepath.Rel(workflowsDir, path)
		skillSlug := buildSkillSlug(module, rel, name)

		content, err := os.ReadFile(path)
		if err != nil {
			report.warn(fmt.Sprintf("read workflow %s: %v", rel, err))
			return nil
		}

		steps := collectFiles(filepath.Join(filepath.Dir(path), "steps"), ".md")
		data := collectFiles(filepath.Join(filepath.Dir(path), "data"), "")
		templates := collectNamedFiles(filepath.Dir(path), "template", "tmpl")

		skill := buildWorkflowSkill(module, skillSlug, string(content), steps, data, templates)
		skillDir := filepath.Join(cfg.vibeHome, "skills", skillSlug)
		skillPath := filepath.Join(skillDir, "SKILL.md")

		if cfg.verbose {
			fmt.Printf("   ⚙️  %s → %s\n", rel, skillSlug)
		}

		if !cfg.dryRun {
			os.MkdirAll(skillDir, 0o755)
		}
		writeFile(cfg, skillPath, skill, report)
		report.skills = append(report.skills, skillSlug)
		return nil
	})
}

func buildWorkflowSkill(module, slug, content string, steps, data, templates []namedContent) string {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }

	// AgentSkills spec frontmatter
	w("---\n")
	w("name: %s\n", slug)
	w("description: \"BMAD %s workflow — auto-generated by bmad2vibe\"\n", strings.ToUpper(module))
	w("license: MIT\n")
	w("user-invocable: true\n")
	w("allowed-tools:\n")
	for _, t := range []string{"read_file", "write_file", "search_replace", "grep", "bash", "ask_user_question", "list_dir"} {
		w("  - %s\n", t)
	}
	w("---\n\n")

	w("> Auto-generated by bmad2vibe from BMAD %s module.\n", strings.ToUpper(module))
	w("> `{project-root}` → cwd | `{output_folder}` → `_bmad-output/`\n")
	w("> `{planning_artifacts}` → `_bmad-output/planning-artifacts/`\n")
	w("> When instructions say \"load workflow engine\", follow steps sequentially.\n\n")

	w("%s\n", content)

	if len(steps) > 0 {
		w("\n---\n\n# Workflow Steps\n\n")
		w("Execute these steps in order.\n\n")
		for _, s := range steps {
			w("## %s\n\n%s\n\n", s.name, s.content)
		}
	}

	if len(templates) > 0 {
		w("\n---\n\n# Templates\n\n")
		for _, t := range templates {
			lang := strings.TrimPrefix(filepath.Ext(t.name), ".")
			if lang == "md" {
				lang = "markdown"
			}
			w("## Template: %s\n\n```%s\n%s\n```\n\n", t.name, lang, t.content)
		}
	}

	if len(data) > 0 {
		w("\n---\n\n# Data Files\n\n")
		for _, d := range data {
			lang := strings.TrimPrefix(filepath.Ext(d.name), ".")
			w("## Data: %s\n\n```%s\n%s\n```\n\n", d.name, lang, d.content)
		}
	}

	return b.String()
}

// --- Phase 3: Task/tool → skill ---

func convertTasks(cfg *config, module, methodDir string, report *conversionReport) {
	tasksDir := filepath.Join(methodDir, "src", "modules", module, "tasks")
	if !dirExists(tasksDir) {
		return
	}

	entries, _ := os.ReadDir(tasksDir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		skillSlug := fmt.Sprintf("bmad-%s-task-%s", module, slug)

		content, err := os.ReadFile(filepath.Join(tasksDir, e.Name()))
		if err != nil {
			continue
		}

		var b strings.Builder
		w := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }
		w("---\n")
		w("name: %s\n", skillSlug)
		w("description: \"BMAD %s task — auto-generated by bmad2vibe\"\n", strings.ToUpper(module))
		w("license: MIT\n")
		w("user-invocable: true\n")
		w("allowed-tools:\n")
		w("  - read_file\n  - write_file\n  - grep\n  - bash\n  - ask_user_question\n  - list_dir\n")
		w("---\n\n")
		w("> BMAD %s task. `{project-root}` → cwd.\n\n", strings.ToUpper(module))
		w("%s\n", string(content))

		skillDir := filepath.Join(cfg.vibeHome, "skills", skillSlug)
		skillPath := filepath.Join(skillDir, "SKILL.md")

		if cfg.verbose {
			fmt.Printf("   🔧 %s/%s → %s\n", module, slug, skillSlug)
		}

		if !cfg.dryRun {
			os.MkdirAll(skillDir, 0o755)
		}
		writeFile(cfg, skillPath, b.String(), report)
		report.skills = append(report.skills, skillSlug)
	}
}

// --- Phase 4: Workflow shortcut agents ---
// Lightweight agents for direct workflow invocation: `vibe --agent bmad-bmm-create-prd`

func generateWorkflowAgents(cfg *config, module, methodDir string, report *conversionReport) {
	workflowsDir := filepath.Join(methodDir, "src", "modules", module, "workflows")
	if !dirExists(workflowsDir) {
		return
	}

	filepath.Walk(workflowsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		if !strings.HasPrefix(name, "workflow") || !strings.HasSuffix(name, ".md") {
			return nil
		}

		rel, _ := filepath.Rel(workflowsDir, path)
		skillSlug := buildSkillSlug(module, rel, name)
		shortName := strings.TrimPrefix(skillSlug, fmt.Sprintf("bmad-%s-", module))
		agentSlug := fmt.Sprintf("bmad-%s-%s", module, shortName)

		// Don't overwrite persona agents from Phase 1
		tomlPath := filepath.Join(cfg.vibeHome, "agents", agentSlug+".toml")
		if fileExists(tomlPath) {
			return nil
		}

		title := toTitle(shortName)
		safety := workflowSafety(shortName)
		tools := safetyToolsMap[safety]

		var toml strings.Builder
		tw := func(f string, a ...any) { fmt.Fprintf(&toml, f, a...) }
		tw("# Auto-generated workflow shortcut agent by bmad2vibe\n")
		tw("# Runs workflow %s directly.\n\n", skillSlug)
		tw("display_name = %q\n", "BMAD "+title)
		tw("description = %q\n", fmt.Sprintf("BMAD %s workflow: %s", strings.ToUpper(module), title))
		tw("safety = %q\n", safety)
		tw("auto_approve = %v\n", safety != "destructive")
		tw("system_prompt_id = %q\n", agentSlug)
		tw("\nenabled_tools = [%s]\n", joinQuoted(tools))

		var prompt strings.Builder
		pw := func(f string, a ...any) { fmt.Fprintf(&prompt, f, a...) }
		pw("# BMAD Workflow: %s\n\n", title)
		pw("> Workflow shortcut agent — auto-generated by bmad2vibe.\n\n")
		pw("## Instructions\n\n")
		pw("1. Read `~/.vibe/skills/%s/SKILL.md`\n", skillSlug)
		pw("2. Follow all instructions sequentially\n")
		pw("3. Substitute `{project-root}` → cwd\n")
		pw("4. Substitute `{output_folder}` → `_bmad-output/`\n")
		pw("5. Substitute `{planning_artifacts}` → `_bmad-output/planning-artifacts/`\n")
		pw("6. Use `ask_user_question` for interactive prompts\n\n")
		pw("Skill slug: `%s`\n", skillSlug)

		promptPath := filepath.Join(cfg.vibeHome, "prompts", agentSlug+".md")

		if cfg.verbose {
			fmt.Printf("   🎯 %s → shortcut to %s\n", agentSlug, skillSlug)
		}

		writeFile(cfg, tomlPath, toml.String(), report)
		writeFile(cfg, promptPath, prompt.String(), report)
		report.agents = append(report.agents, agentSlug+" (workflow)")
		report.prompts = append(report.prompts, agentSlug)
		return nil
	})
}

// --- Phase 5: Copy data ---

func copyModuleData(cfg *config, module, methodDir string, report *conversionReport) {
	for _, sub := range []string{"data", "docs"} {
		src := filepath.Join(methodDir, "src", "modules", module, sub)
		if !dirExists(src) {
			continue
		}
		dest := filepath.Join(cfg.vibeHome, "skills", fmt.Sprintf("bmad-%s-%s", module, sub))

		if cfg.dryRun {
			fmt.Printf("   [DRY] Would copy %s → %s\n", sub, dest)
			continue
		}

		if err := copyDir(src, dest); err != nil {
			report.warn(fmt.Sprintf("copy %s/%s: %v", module, sub, err))
		} else if cfg.verbose {
			fmt.Printf("   📄 %s/%s copied\n", module, sub)
		}
	}
}

// --- Phase 6: AGENTS.md ---

func generateAgentsMD(cfg *config, report *conversionReport) {
	if cfg.dryRun {
		fmt.Println("   [DRY] Would generate AGENTS.md")
		return
	}

	agentsDir := filepath.Join(cfg.vibeHome, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return
	}

	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }

	w("# AGENTS.md — BMAD Method for Mistral Vibe\n\n")
	w("Auto-generated by bmad2vibe. Copy to your project root for Vibe AGENTS.md support.\n\n")
	w("## Persona Agents\n\n")
	w("Launch: `vibe --agent <name>` or `Shift+Tab` in interactive mode.\n\n")
	w("| Agent | Command | Description |\n")
	w("|---|---|---|\n")

	var wfRows []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".toml")
		data, _ := os.ReadFile(filepath.Join(agentsDir, e.Name()))
		content := string(data)
		dn := extractTOMLVal(content, "display_name")
		desc := extractTOMLVal(content, "description")

		if strings.Contains(content, "workflow shortcut") {
			wfRows = append(wfRows, fmt.Sprintf("| %s | `vibe --agent %s` | %s |", dn, slug, desc))
		} else {
			w("| %s | `vibe --agent %s` | %s |\n", dn, slug, desc)
		}
	}

	if len(wfRows) > 0 {
		w("\n## Workflow Shortcut Agents\n\n")
		w("| Agent | Command | Description |\n")
		w("|---|---|---|\n")
		for _, row := range wfRows {
			w("%s\n", row)
		}
	}

	path := filepath.Join(cfg.vibeHome, "AGENTS.md")
	writeFile(cfg, path, b.String(), report)
	if cfg.verbose {
		fmt.Println("   📝 AGENTS.md generated")
	}
}

// --- Phase 7: Validation ---

func validate(cfg *config, report *conversionReport) {
	if cfg.dryRun {
		fmt.Println("   (skipped in dry-run)")
		return
	}

	agentsDir := filepath.Join(cfg.vibeHome, "agents")
	promptsDir := filepath.Join(cfg.vibeHome, "prompts")
	skillsDir := filepath.Join(cfg.vibeHome, "skills")

	tomlFiles, _ := filepath.Glob(filepath.Join(agentsDir, "bmad-*.toml"))
	promptFiles, _ := filepath.Glob(filepath.Join(promptsDir, "bmad-*.md"))

	// 1. TOML → prompt cross-ref + required fields + valid safety
	for _, tp := range tomlFiles {
		data, _ := os.ReadFile(tp)
		c := string(data)
		base := filepath.Base(tp)

		pid := extractTOMLVal(c, "system_prompt_id")
		if pid == "" {
			report.err(fmt.Sprintf("%s: missing system_prompt_id", base))
			continue
		}
		if !fileExists(filepath.Join(promptsDir, pid+".md")) {
			report.err(fmt.Sprintf("%s: prompt %s.md not found", base, pid))
		}

		for _, f := range []string{"display_name", "description", "safety", "enabled_tools"} {
			if !strings.Contains(c, f+" =") && !strings.Contains(c, f+"=") {
				report.err(fmt.Sprintf("%s: missing field %q", base, f))
			}
		}

		safety := extractTOMLVal(c, "safety")
		valid := map[string]bool{"safe": true, "neutral": true, "destructive": true, "yolo": true}
		if !valid[safety] {
			report.err(fmt.Sprintf("%s: invalid safety %q", base, safety))
		}
	}

	// 2. Prompt size
	for _, p := range promptFiles {
		info, _ := os.Stat(p)
		if info != nil && info.Size() < 50 {
			report.warn(fmt.Sprintf("%s: suspiciously small (%d bytes)", filepath.Base(p), info.Size()))
		}
	}

	// 3. Orphaned prompts
	for _, p := range promptFiles {
		slug := strings.TrimSuffix(filepath.Base(p), ".md")
		if !fileExists(filepath.Join(agentsDir, slug+".toml")) {
			report.warn(fmt.Sprintf("orphaned prompt: %s.md", slug))
		}
	}

	// 4. Skill dirs have SKILL.md (except data/docs dirs)
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "bmad-") {
				continue
			}
			if !fileExists(filepath.Join(skillsDir, e.Name(), "SKILL.md")) {
				if !strings.HasSuffix(e.Name(), "-data") && !strings.HasSuffix(e.Name(), "-docs") {
					report.warn(fmt.Sprintf("skill %s: missing SKILL.md", e.Name()))
				}
			}
		}
	}

	// 5. Workflow shortcut → skill exists
	for _, tp := range tomlFiles {
		data, _ := os.ReadFile(tp)
		c := string(data)
		if !strings.Contains(c, "workflow shortcut") {
			continue
		}
		pid := extractTOMLVal(c, "system_prompt_id")
		pData, _ := os.ReadFile(filepath.Join(promptsDir, pid+".md"))
		re := regexp.MustCompile("Skill slug: `([^`]+)`")
		m := re.FindStringSubmatch(string(pData))
		if len(m) >= 2 && !dirExists(filepath.Join(skillsDir, m[1])) {
			report.err(fmt.Sprintf("%s: skill %s not found", filepath.Base(tp), m[1]))
		}
	}

	// Counts
	skillCount := 0
	if entries, _ := os.ReadDir(skillsDir); entries != nil {
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "bmad-") {
				skillCount++
			}
		}
	}
	fmt.Printf("   Agents: %d | Prompts: %d | Skills: %d\n", len(tomlFiles), len(promptFiles), skillCount)
}

// --- Report ---

func printReport(cfg *config, report *conversionReport) {
	fmt.Println("\n" + strings.Repeat("═", 60))
	fmt.Println("📊 Conversion Report")
	fmt.Println(strings.Repeat("═", 60))

	agents := unique(report.agents)
	skills := unique(report.skills)
	sort.Strings(agents)
	sort.Strings(skills)

	persona := filter(agents, func(s string) bool { return !strings.Contains(s, "(workflow)") })
	wf := filter(agents, func(s string) bool { return strings.Contains(s, "(workflow)") })

	fmt.Printf("\n✅ Persona agents: %d\n", len(persona))
	for _, a := range persona {
		fmt.Printf("   • %s\n", a)
	}
	fmt.Printf("✅ Workflow agents: %d\n", len(wf))

	fmt.Printf("✅ Skills:          %d\n", len(skills))
	for _, s := range skills {
		fmt.Printf("   • %s\n", s)
	}

	if len(report.warnings) > 0 {
		fmt.Printf("\n⚠️  Warnings: %d\n", len(report.warnings))
		for _, w := range report.warnings {
			fmt.Printf("   ⚠️  %s\n", w)
		}
	}

	if len(report.errors) > 0 {
		fmt.Printf("\n❌ Errors: %d\n", len(report.errors))
		for _, e := range report.errors {
			fmt.Printf("   ❌ %s\n", e)
		}
		os.Exit(1)
	}

	fmt.Println("\n🎉 All checks passed!")
	fmt.Println("\n📖 Usage:")
	if len(persona) > 0 {
		fmt.Printf("  vibe --agent %s\n", persona[0])
	}
	fmt.Printf("\n  AGENTS.md: %s/AGENTS.md\n", cfg.vibeHome)
	fmt.Println("  → Copy to project root for AGENTS.md support")
}

// --- Helpers ---

func extractAgentMeta(slug, raw string) agentMeta {
	return agentMeta{
		Slug:        slug,
		Name:        extractXMLAttr(raw, "name"),
		Title:       extractXMLAttr(raw, "title"),
		Icon:        extractXMLAttr(raw, "icon"),
		Description: extractXMLAttr(raw, "description"),
	}
}

func extractXMLAttr(raw, attr string) string {
	tagEnd := strings.Index(raw, ">")
	if tagEnd == -1 {
		return ""
	}
	re := regexp.MustCompile(fmt.Sprintf(`%s="([^"]*)"`, regexp.QuoteMeta(attr)))
	m := re.FindStringSubmatch(raw[:tagEnd+1])
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func extractTOMLVal(content, key string) string {
	re := regexp.MustCompile(fmt.Sprintf(`%s\s*=\s*"([^"]*)"`, regexp.QuoteMeta(key)))
	m := re.FindStringSubmatch(content)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func safetyForAgent(slug string) string {
	if s, ok := agentSafetyMap[slug]; ok {
		return s
	}
	return "neutral"
}

func workflowSafety(name string) string {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "dev") || strings.Contains(lower, "implement") {
		return "destructive"
	}
	return "neutral"
}

func buildSkillSlug(module, rel, name string) string {
	dir := filepath.Dir(rel)
	parts := strings.Split(dir, string(filepath.Separator))
	slug := "bmad-" + module
	for _, p := range parts {
		p = strings.TrimPrefix(p, "workflow-")
		p = strings.TrimPrefix(p, "bmad-")
		if p != "." && p != "" {
			slug += "-" + p
		}
	}
	if strings.HasPrefix(name, "workflow-") {
		suffix := strings.TrimSuffix(strings.TrimPrefix(name, "workflow-"), ".md")
		slug += "-" + suffix
	}
	return slug
}

func toTitle(s string) string {
	words := strings.Split(strings.ReplaceAll(s, "-", " "), " ")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

type namedContent struct {
	name    string
	content string
}

func collectFiles(dir, extFilter string) []namedContent {
	if !dirExists(dir) {
		return nil
	}
	entries, _ := os.ReadDir(dir)
	var result []namedContent
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if extFilter != "" && !strings.HasSuffix(e.Name(), extFilter) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		result = append(result, namedContent{name: e.Name(), content: string(data)})
	}
	return result
}

func collectNamedFiles(dir string, substrings ...string) []namedContent {
	if !dirExists(dir) {
		return nil
	}
	entries, _ := os.ReadDir(dir)
	var result []namedContent
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		lower := strings.ToLower(e.Name())
		match := false
		for _, sub := range substrings {
			if strings.Contains(lower, sub) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		result = append(result, namedContent{name: e.Name(), content: string(data)})
	}
	return result
}

func writeFile(cfg *config, path, content string, report *conversionReport) {
	if cfg.dryRun {
		fmt.Printf("   [DRY] %s\n", path)
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		report.err(fmt.Sprintf("write %s: %v", path, err))
	}
}

func copyDir(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, _ := os.ReadFile(path)
		os.MkdirAll(filepath.Dir(target), 0o755)
		return os.WriteFile(target, data, 0o644)
	})
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func joinQuoted(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, ", ")
}

func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func unique(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var result []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

func filter(ss []string, fn func(string) bool) []string {
	var result []string
	for _, s := range ss {
		if fn(s) {
			result = append(result, s)
		}
	}
	return result
}

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"gotodo/internal/loops"
	"gotodo/internal/task"
)

const loopsUsage = `usage: gotodo loops [status]
       gotodo loops on|off [-project]
       gotodo loops set <stage> -agent claude|codex|opencode [-model M] [-skill S] [-prompt P] [-permissions readonly|edits|full] [-enabled=false] [-project]
       gotodo loops unset <stage> [-project]
       gotodo loops logs <id> [-stage S]
       gotodo loops stop <id>
       gotodo loops dispatch            start runs for new card moves and drain the queue now
       gotodo loops run <id> <stage>    (internal: what a dispatched loop executes)`

// runLoops shows or changes stage loops: coding agents started when a card
// enters a column.
func runLoops(path string, args []string) error {
	if len(args) == 0 {
		return runLoopsStatus(path)
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Println(loopsUsage)
		return nil
	case "status":
		return runLoopsStatus(path)
	case "on", "off":
		return runLoopsToggle(path, args[0] == "on", args[1:])
	case "set":
		return runLoopsSet(path, args[1:])
	case "unset":
		return runLoopsUnset(path, args[1:])
	case "logs":
		return runLoopsLogs(path, args[1:])
	case "stop":
		return runLoopsStop(path, args[1:])
	case "dispatch":
		if len(args) != 1 {
			return errors.New("usage: gotodo loops dispatch")
		}
		launches, err := loops.Dispatch(path, time.Now())
		for _, l := range launches {
			fmt.Printf("started %s %s run %d (%s, pid %d) · log %s\n", shortID(l.TaskID), l.Stage, l.N, l.Agent, l.PID, l.Log)
		}
		if err == nil && len(launches) == 0 {
			fmt.Println("nothing to start")
		}
		return err
	case "run":
		if len(args) != 3 {
			return errors.New("usage: gotodo loops run <id> <stage>")
		}
		return loops.Run(path, args[1], args[2], os.Stdout)
	}
	return fmt.Errorf("unknown loops command %q\n%s", args[0], loopsUsage)
}

// refuseLoopConfig keeps agents, and loops above all, from changing what
// loops run.
func refuseLoopConfig() error {
	if loops.InLoop() {
		return fmt.Errorf("a loop cannot reconfigure loops (%s=1)", loops.LoopEnv)
	}
	if agent := task.AgentFromEnv(); agent != "" {
		return fmt.Errorf("agents cannot reconfigure loops (%s=%s); ask a human to run it", task.AgentEnv, agent)
	}
	return nil
}

// loopsTarget is the config file on/off/set/unset write: the project file
// with -project or when it exists, else the global one.
func loopsTarget(path string, project bool) (string, error) {
	proj := loops.ProjectPath(path)
	if project {
		if proj == "" {
			return "", errors.New("-project needs a project board (.deadline/board.json); run gotodo init")
		}
		return proj, nil
	}
	if proj != "" {
		if _, err := os.Stat(proj); err == nil {
			return proj, nil
		}
	}
	return loops.GlobalPath()
}

// readTarget reads the file to change. A new project file starts as a copy
// of the effective config, so creating it changes only what was asked.
func readTarget(path, target string) (loops.Config, bool, error) {
	c, err := loops.ReadFile(target)
	if err == nil {
		return c, false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return loops.Config{}, false, err
	}
	if target == loops.ProjectPath(path) {
		eff, _, err := loops.Load(path)
		return eff, true, err
	}
	return loops.Config{}, true, nil
}

func loopsFlagSet(name string) (*flag.FlagSet, *bool) {
	fs := flag.NewFlagSet("loops "+name, flag.ContinueOnError)
	project := fs.Bool("project", false, "write this project's .deadline/loops.json")
	return fs, project
}

func runLoopsToggle(path string, on bool, args []string) error {
	fs, project := loopsFlagSet("on|off")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q\n%s", fs.Arg(0), loopsUsage)
	}
	if err := refuseLoopConfig(); err != nil {
		return err
	}
	target, err := loopsTarget(path, *project)
	if err != nil {
		return err
	}
	c, _, err := readTarget(path, target)
	if err != nil {
		return err
	}
	was := c.Enabled
	c.Enabled = on
	if err := loops.WriteFile(target, c); err != nil {
		return err
	}
	if on && !was {
		// Only moves from now on start loops.
		if err := loops.Prime(path, time.Now()); err != nil {
			return err
		}
	}
	state := "off"
	if on {
		state = "on"
	}
	fmt.Printf("loops: %s (%s)\n", state, target)
	if on && len(c.Stages) == 0 {
		fmt.Println("no stages yet: gotodo loops set <stage> -agent claude -skill <skill>")
	}
	return nil
}

func runLoopsSet(path string, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
			fmt.Println(loopsUsage)
			return nil
		}
		return errors.New(loopsUsage)
	}
	stage := args[0]
	fs, project := loopsFlagSet("set")
	agent := fs.String("agent", "", "claude, codex or opencode")
	model := fs.String("model", "", "model passed to the agent CLI (blank: its default)")
	skill := fs.String("skill", "", "skill the prompt invokes")
	prompt := fs.String("prompt", "", "prompt template ({skill} {id} {title} {stage} {board})")
	perms := fs.String("permissions", "", "readonly, edits (default) or full")
	enabled := fs.Bool("enabled", true, "run this stage (false pauses it)")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q\n%s", fs.Arg(0), loopsUsage)
	}
	if err := refuseLoopConfig(); err != nil {
		return err
	}
	b, err := loadBoard(path)
	if err != nil {
		return err
	}
	if _, err := boardStatus(b, stage); err != nil {
		return err
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	target, err := loopsTarget(path, *project)
	if err != nil {
		return err
	}
	c, created, err := readTarget(path, target)
	if err != nil {
		return err
	}
	st := c.Stages[stage]
	if set["agent"] {
		st.Agent = *agent
	}
	if set["model"] {
		st.Model = *model
	}
	if set["skill"] {
		st.Skill = *skill
	}
	if set["prompt"] {
		st.Prompt = *prompt
	}
	if set["permissions"] {
		st.Permissions = *perms
	}
	if set["enabled"] {
		st.Enabled = nil
		if !*enabled {
			off := false
			st.Enabled = &off
		}
	}
	if st.Agent == "" {
		return fmt.Errorf("-agent is required for a new stage (%s)", strings.Join(loops.Agents, ", "))
	}
	if c.Stages == nil {
		c.Stages = map[string]loops.Stage{}
	}
	c.Stages[stage] = st
	if err := loops.WriteFile(target, c); err != nil {
		return err
	}
	if created {
		fmt.Printf("created %s\n", target)
	}
	fmt.Printf("loop %s: %s · model %s · skill %s · permissions %s\n", stage, st.Agent, dash(st.Model), dash(st.Skill), st.Perm())
	if st.Skill != "" && !loops.Hooks.HasSkill(st.Agent, st.Skill) {
		fmt.Printf("warning: skill %s not found for %s; runs will be skipped until it is installed (gotodo setup)\n", st.Skill, st.Agent)
	}
	if st.Perm() == loops.PermFull {
		fmt.Println("warning: full skips every permission prompt; use it only in a sandboxed or throwaway checkout")
	}
	if !c.Enabled {
		fmt.Println("loops are off: gotodo loops on")
	}
	return nil
}

func runLoopsUnset(path string, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New(loopsUsage)
	}
	stage := args[0]
	fs, project := loopsFlagSet("unset")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if err := refuseLoopConfig(); err != nil {
		return err
	}
	target, err := loopsTarget(path, *project)
	if err != nil {
		return err
	}
	c, _, err := readTarget(path, target)
	if err != nil {
		return err
	}
	if _, ok := c.Stages[stage]; !ok {
		return fmt.Errorf("no loop for stage %q in %s", stage, target)
	}
	delete(c.Stages, stage)
	if err := loops.WriteFile(target, c); err != nil {
		return err
	}
	fmt.Printf("removed loop %s (%s)\n", stage, target)
	return nil
}

func runLoopsLogs(path string, args []string) error {
	fs := flag.NewFlagSet("loops logs", flag.ContinueOnError)
	stage := fs.String("stage", "", "only this stage's runs")
	id, help, err := coordIDArgs(fs, args, "usage: gotodo loops logs <id> [-stage S]")
	if help || err != nil {
		return err
	}
	b, err := loadBoard(path)
	if err != nil {
		return err
	}
	t, err := resolveID(b, id)
	if err != nil {
		return err
	}
	rec, err := loops.LatestLog(path, t.ID, *stage)
	if err != nil {
		return fmt.Errorf("%s: %w", shortID(t.ID), err)
	}
	f, err := os.Open(rec.Log)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(os.Stdout, f)
	return err
}

func runLoopsStop(path string, args []string) error {
	fs := flag.NewFlagSet("loops stop", flag.ContinueOnError)
	id, help, err := coordIDArgs(fs, args, "usage: gotodo loops stop <id>")
	if help || err != nil {
		return err
	}
	if loops.InLoop() {
		return fmt.Errorf("a loop cannot stop loops (%s=1)", loops.LoopEnv)
	}
	b, err := loadBoard(path)
	if err != nil {
		return err
	}
	t, err := resolveID(b, id)
	if err != nil {
		return err
	}
	stopped, err := loops.Stop(path, t.ID, time.Now())
	if err != nil {
		return err
	}
	if len(stopped) == 0 {
		fmt.Printf("%s %s has no live loop runs\n", shortID(t.ID), t.Title)
		return nil
	}
	for _, r := range stopped {
		fmt.Printf("stopped %s %s run %d (%s)\n", shortID(r.TaskID), r.Stage, r.N, r.Agent)
	}
	return nil
}

func runLoopsStatus(path string) error {
	c, from, err := loops.Load(path)
	if err != nil {
		return err
	}
	b, err := loadBoard(path)
	if err != nil {
		return err
	}
	if from == "" {
		fmt.Println("loops: off (no config)")
		fmt.Println("start: gotodo loops set indev -agent claude -skill <skill>, then gotodo loops on")
		return nil
	}
	state := "off"
	if c.Enabled {
		state = "on"
	}
	parallel, perTask := c.Limits()
	fmt.Printf("loops: %s · config %s\n", state, from)
	fmt.Printf("limits: %d parallel · %d runs per task per stage · timeout %dm\n", parallel, perTask, int(c.Timeout().Minutes()))
	if len(c.Stages) == 0 {
		fmt.Println("no stages configured")
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "STAGE\tAGENT\tMODEL\tSKILL\tPERMISSIONS\tSKILL INSTALLED")
		for _, name := range c.StageNames() {
			st := c.Stages[name]
			installed := "✓"
			if st.Skill == "" {
				installed = "-"
			} else if !loops.Hooks.HasSkill(st.Agent, st.Skill) {
				installed = "✗"
			}
			label := name
			if !st.On() {
				label += " (paused)"
			}
			if !b.HasStatus(task.Status(name)) {
				label += " (not a column here)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", label, st.Agent, dash(st.Model), dash(st.Skill), st.Perm(), installed)
		}
		w.Flush()
	}
	r, err := loops.Status(path)
	if err != nil {
		return err
	}
	title := func(id string) string {
		if t, ok := b.TaskByID(id); ok {
			return t.Title
		}
		return "(deleted)"
	}
	if len(r.Running) == 0 {
		fmt.Println("running: none")
	} else {
		fmt.Println("running:")
		for _, run := range r.Running {
			fmt.Printf("  %s %s · %s %s · %s · started %s · log %s\n", shortID(run.TaskID), title(run.TaskID),
				run.Stage, fmt.Sprintf("run %d", run.N), run.Agent, run.Started.In(time.Local).Format(dateLayout+" 15:04"), run.Log)
		}
	}
	if len(r.Queued) > 0 {
		fmt.Println("queued:")
		for _, q := range r.Queued {
			fmt.Printf("  %s %s · %s · since %s\n", shortID(q.TaskID), title(q.TaskID), q.Stage,
				q.QueuedAt.In(time.Local).Format(dateLayout+" 15:04"))
		}
	}
	fmt.Printf("events: %s\n", loops.EventLogPath(path))
	return nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

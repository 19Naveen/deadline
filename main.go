// Command gotodo is a keyboard-driven terminal kanban board.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"gotodo/internal/task"
	"gotodo/internal/ui"
)

func main() {
	defaultPath, err := task.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gotodo:", err)
		os.Exit(1)
	}
	path := flag.String("file", defaultPath, "path to the tasks JSON file")
	flag.Parse()

	board, err := task.Load(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gotodo:", err)
		os.Exit(1)
	}

	// Tidy the board before the first frame: anything done for two weeks
	// belongs in the archive, not in the done column.
	board.SweepArchive(time.Now())

	p := tea.NewProgram(ui.NewApp(board), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gotodo:", err)
		os.Exit(1)
	}
	// Final save covers any state the dirty path missed. Skipped when the
	// board isn't dirty so a read-only session can't clobber another
	// instance's save with a stale snapshot.
	if board.Dirty() {
		if err := board.Save(); err != nil {
			fmt.Fprintln(os.Stderr, "gotodo: save failed:", err)
			os.Exit(1)
		}
	}
}

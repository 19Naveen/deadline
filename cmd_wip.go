package main

import (
	"errors"
	"fmt"
	"strconv"
)

const wipUsage = "usage: gotodo wip [<status> <limit>]   (limit 0 clears)"

// runWIP prints each column's card count and WIP limit, or with a status and
// a number sets that column's limit (0 clears it).
func runWIP(path string, args []string) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Println(wipUsage)
		return nil
	}
	if len(args) != 0 && len(args) != 2 {
		return errors.New(wipUsage)
	}
	board, err := loadBoard(path)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		for _, s := range board.Statuses() {
			count, limit, over := board.OverWIP(s)
			switch {
			case limit == 0:
				fmt.Printf("%s: %d (no limit)\n", s, count)
			case over:
				fmt.Printf("%s: %d/%d over limit\n", s, count, limit)
			default:
				fmt.Printf("%s: %d/%d\n", s, count, limit)
			}
		}
		return nil
	}
	s, err := boardStatus(board, args[0])
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(args[1])
	if err != nil || n < 0 {
		return fmt.Errorf("bad limit %q (want a whole number, 0 clears)", args[1])
	}
	board.SetWIPLimit(s, n)
	if err := board.Save(); err != nil {
		return err
	}
	if n == 0 {
		fmt.Printf("cleared WIP limit for %s\n", s)
		return nil
	}
	count, _, over := board.OverWIP(s)
	fmt.Printf("WIP limit for %s: %d (now %d)\n", s, n, count)
	if over {
		printWIPWarnings(board, s)
	}
	return nil
}

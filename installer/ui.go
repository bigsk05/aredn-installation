package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// chooseDisk shows an arrow-key menu for selecting the target disk (enters raw mode, restores on exit).
func chooseDisk(disks []Disk) (*Disk, error) {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return nil, err
	}
	defer term.Restore(fd, old)

	sel := 0
	first := true
	var b [1]byte
	for {
		renderMenu(disks, sel, first)
		first = false

		if _, err := os.Stdin.Read(b[:]); err != nil {
			continue
		}
		switch b[0] {
		case 'q', 0x03: // q / Ctrl-C
			return nil, fmt.Errorf("cancelled by user")
		case '\r', '\n', ' ':
			return &disks[sel], nil
		case 'j':
			sel = (sel + 1) % len(disks)
		case 'k':
			sel = (sel + len(disks) - 1) % len(disks)
		case 0x1b: // ESC [ A/B (arrow keys)
			seq := make([]byte, 2)
			if _, err := os.Stdin.Read(seq); err != nil || seq[0] != '[' {
				continue
			}
			switch seq[1] {
			case 'A': // ↑
				sel = (sel + len(disks) - 1) % len(disks)
			case 'B': // ↓
				sel = (sel + 1) % len(disks)
			}
		}
	}
}

func renderMenu(disks []Disk, sel int, first bool) {
	if !first {
		// redraw from the top
		fmt.Print("\x1b[H")
	}
	clear := ""
	if !first {
		clear = "\x1b[2J"
	}
	fmt.Print("\x1b[H" + clear)
	fmt.Println("== AREDN x86_64 Installer - select target disk ==")
	fmt.Println("(use arrow keys or j/k to move, Enter to select, q to quit)")
	fmt.Println()
	for i, d := range disks {
		marker := "  "
		if i == sel {
			marker = "> "
		}
		fmt.Printf("%s[%2d] %-14s %10s  %s\n", marker, i+1, d.Path, humanSize(d.Size), d.Model)
	}
}

// readLine reads one line of input (cooked mode, with echo).
func readLine(prompt string) (string, error) {
	fmt.Print(prompt + " ")
	r := bufio.NewReader(os.Stdin)
	s, err := r.ReadString('\n')
	return strings.TrimSpace(s), err
}

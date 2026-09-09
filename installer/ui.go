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
	var b [1]byte
	for {
		renderMenu(disks, sel)

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
			case 'A': // up
				sel = (sel + len(disks) - 1) % len(disks)
			case 'B': // down
				sel = (sel + 1) % len(disks)
			}
		}
	}
}

// diskWidths returns the column widths needed to align all disk rows.
func diskWidths(disks []Disk) (pathW, sizeW int) {
	pathW, sizeW = 8, 6
	for _, d := range disks {
		if l := len(d.Path); l > pathW {
			pathW = l
		}
		if l := len(humanSize(d.Size)); l > sizeW {
			sizeW = l
		}
	}
	return pathW, sizeW
}

// diskRow renders one aligned disk row (path | size | model).
// The model is truncated so the line never wraps on narrow consoles.
func diskRow(d Disk, pathW, sizeW, width int) string {
	row := fmt.Sprintf("%-*s  %*s", pathW, d.Path, sizeW, humanSize(d.Size))
	model := strings.TrimSpace(d.Model)
	if model != "" {
		avail := width - len(row) - 2
		if avail < 10 { // no room for a model on this line
			return row
		}
		if len(model) > avail {
			model = model[:avail-3] + "..."
		}
		row += "  " + model
	}
	return strings.TrimRight(row, " ")
}

// renderMenu prints the disk selection menu. Every render clears the screen
// first so boot/kernel output cannot interfere. All lines are terminated with
// explicit \r\n: inside raw mode OPOST/ONLCR is off, so a bare "\n" would only
// move down without returning to column 0, staggering every line to the right.
func renderMenu(disks []Disk, sel int) {
	fd := int(os.Stdin.Fd())
	width := 80
	if w, _, err := term.GetSize(fd); err == nil && w >= 60 {
		width = w
	}
	pathW, sizeW := diskWidths(disks)

	// clear the whole screen and move the cursor home
	fmt.Print("\x1b[2J\x1b[H")
	line := func(s string) { fmt.Print(s + "\r\n") }
	line("AREDN x86_64 Installer - select target disk")
	line("")
	for i, d := range disks {
		row := diskRow(d, pathW, sizeW, width-2)
		if i == sel {
			// reverse video highlights the selected row (works on VGA and serial consoles)
			line(fmt.Sprintf("> [%2d] \x1b[7m%s\x1b[0m", i+1, row))
		} else {
			line(fmt.Sprintf("  [%2d] %s", i+1, row))
		}
	}
	line("")
	fmt.Print("(arrow keys or j/k: move, Enter: select, q: quit)")
}

// readLine reads one line of input (cooked mode, with echo).
func readLine(prompt string) (string, error) {
	fmt.Print(prompt + " ")
	r := bufio.NewReader(os.Stdin)
	s, err := r.ReadString('\n')
	return strings.TrimSpace(s), err
}

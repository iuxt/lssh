package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

var errSelectionCanceled = errors.New("selection canceled")

type menuItem struct {
	title    string
	details  string
	selected bool
}

func chooseSourceInteractive(sources []ConfigSource) (ConfigSource, error) {
	items := make([]menuItem, 0, len(sources))
	for _, source := range sources {
		profileCount := len(source.Config.Profiles)
		items = append(items, menuItem{
			title:   source.Name,
			details: fmt.Sprintf("%s (%d profile%s)", source.Path, profileCount, pluralSuffix(profileCount)),
		})
	}

	index, err := runMenu("Select a config file", items)
	if err != nil {
		return ConfigSource{}, err
	}
	return sources[index], nil
}

func chooseProfileInteractive(source ConfigSource) (SSHProfile, string, error) {
	names := sortedProfileNames(source.Config)
	items := make([]menuItem, 0, len(names))
	for _, name := range names {
		profile := source.Config.Profiles[name]
		detail := fmt.Sprintf("%s@%s:%d", profile.User, profile.Host, profile.Port)
		if name == source.Config.DefaultProfile {
			detail += " [default]"
		}
		items = append(items, menuItem{
			title:    name,
			details:  detail,
			selected: name == source.Config.DefaultProfile,
		})
	}

	index, err := runMenu("Select a profile", items)
	if err != nil {
		return SSHProfile{}, "", err
	}

	name := names[index]
	return source.Config.Profiles[name], name, nil
}

func runMenu(title string, items []menuItem) (int, error) {
	if len(items) == 0 {
		return -1, errors.New("no items to select")
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return -1, errors.New("interactive selection requires a terminal")
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return -1, fmt.Errorf("enable raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	reader := bufio.NewReader(os.Stdin)
	current := 0
	for i, item := range items {
		if item.selected {
			current = i
			break
		}
	}

	hideCursor()
	defer showCursor()

	for {
		renderMenu(title, items, current)
		key, err := readMenuKey(reader)
		if err != nil {
			return -1, err
		}

		switch key {
		case "up":
			if current > 0 {
				current--
			} else {
				current = len(items) - 1
			}
		case "down":
			if current < len(items)-1 {
				current++
			} else {
				current = 0
			}
		case "enter":
			clearScreen()
			return current, nil
		case "cancel":
			clearScreen()
			return -1, errSelectionCanceled
		}
	}
}

func renderMenu(title string, items []menuItem, current int) {
	clearScreen()
	fmt.Fprintf(os.Stdout, "%s\n", title)
	fmt.Fprintln(os.Stdout, "Use ↑/↓ or j/k to move, Enter to confirm, q to quit.")
	fmt.Fprintln(os.Stdout)

	for i, item := range items {
		cursor := "  "
		if i == current {
			cursor = "> "
		}
		fmt.Fprintf(os.Stdout, "%s%s\n", cursor, item.title)
		if item.details != "" {
			fmt.Fprintf(os.Stdout, "  %s\n", item.details)
		}
	}
}

func readMenuKey(reader *bufio.Reader) (string, error) {
	b, err := reader.ReadByte()
	if err != nil {
		return "", err
	}

	switch b {
	case '\r', '\n':
		return "enter", nil
	case 'k', 'K':
		return "up", nil
	case 'j', 'J':
		return "down", nil
	case 'q', 'Q', 0x03:
		return "cancel", nil
	case 0x1b:
		next, err := reader.ReadByte()
		if err != nil {
			return "cancel", nil
		}
		if next != '[' {
			return "cancel", nil
		}
		arrow, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		switch arrow {
		case 'A':
			return "up", nil
		case 'B':
			return "down", nil
		default:
			return "", nil
		}
	default:
		return "", nil
	}
}

func clearScreen() {
	fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
}

func hideCursor() {
	fmt.Fprint(os.Stdout, "\x1b[?25l")
}

func showCursor() {
	fmt.Fprint(os.Stdout, "\x1b[?25h")
}

func pluralSuffix(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func chooseInteractiveTarget(sources []ConfigSource) (ConfigSource, SSHProfile, string, error) {
	source := ConfigSource{}
	var err error

	if len(sources) == 1 {
		source = sources[0]
	} else {
		source, err = chooseSourceInteractive(sources)
		if err != nil {
			return ConfigSource{}, SSHProfile{}, "", err
		}
	}

	profile, name, err := chooseProfileInteractive(source)
	if err != nil {
		return ConfigSource{}, SSHProfile{}, "", err
	}
	return source, profile, name, nil
}

func selectionErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errSelectionCanceled) {
		return "selection canceled"
	}
	return strings.TrimSpace(err.Error())
}

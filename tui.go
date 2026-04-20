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

type menuState struct {
	current    int
	searchMode bool
	query      string
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
	state := menuState{}
	for i, item := range items {
		if item.selected {
			state.current = i
			break
		}
	}

	hideCursor()
	defer showCursor()

	for {
		filtered, visibleIndexes := filteredMenuItems(items, state.query)
		if len(filtered) == 0 {
			state.current = 0
		} else if state.current >= len(filtered) {
			state.current = len(filtered) - 1
		}

		renderMenu(title, filtered, state)
		key, text, err := readMenuKey(reader, state.searchMode)
		if err != nil {
			return -1, err
		}

		switch key {
		case "up":
			if len(filtered) == 0 {
				continue
			}
			if state.current > 0 {
				state.current--
			} else {
				state.current = len(filtered) - 1
			}
		case "down":
			if len(filtered) == 0 {
				continue
			}
			if state.current < len(filtered)-1 {
				state.current++
			} else {
				state.current = 0
			}
		case "enter":
			if state.searchMode {
				state.searchMode = false
				continue
			}
			if len(filtered) == 0 {
				continue
			}
			clearScreen()
			return visibleIndexes[state.current], nil
		case "cancel":
			if state.searchMode {
				state.searchMode = false
				state.query = ""
				state.current = 0
				continue
			}
			clearScreen()
			return -1, errSelectionCanceled
		case "search":
			state.searchMode = true
		case "backspace":
			if state.searchMode && len(state.query) > 0 {
				state.query = state.query[:len(state.query)-1]
				state.current = 0
			}
		case "text":
			if state.searchMode {
				state.query += text
				state.current = 0
			}
		}
	}
}

func renderMenu(title string, items []menuItem, state menuState) {
	clearScreen()
	writeMenuLine(title)
	writeMenuLine("Use ↑/↓ or j/k to move, / to search, Enter to confirm, q to quit.")
	writeMenuLine("")

	if state.searchMode {
		writeMenuLine("Search: /" + state.query)
	} else if state.query != "" {
		writeMenuLine("Filter: /" + state.query)
	} else {
		writeMenuLine("")
	}
	writeMenuLine("")

	if len(items) == 0 {
		writeMenuLine("  No matches")
		return
	}

	for i, item := range items {
		cursor := "  "
		if i == state.current {
			cursor = "> "
		}
		writeMenuLine(cursor + item.title)
		if item.details != "" {
			writeMenuLine("    " + item.details)
		}
	}
}

func readMenuKey(reader *bufio.Reader, searchMode bool) (string, string, error) {
	b, err := reader.ReadByte()
	if err != nil {
		return "", "", err
	}

	switch b {
	case '\r', '\n':
		return "enter", "", nil
	case 'k', 'K':
		if searchMode {
			return "text", string(b), nil
		}
		return "up", "", nil
	case 'j', 'J':
		if searchMode {
			return "text", string(b), nil
		}
		return "down", "", nil
	case '/':
		if searchMode {
			return "text", "/", nil
		}
		return "search", "", nil
	case 'q', 'Q', 0x03:
		if searchMode && b != 0x03 {
			return "text", string(b), nil
		}
		return "cancel", "", nil
	case 0x7f, 0x08:
		if searchMode {
			return "backspace", "", nil
		}
		return "", "", nil
	case 0x1b:
		next, err := reader.ReadByte()
		if err != nil {
			if searchMode {
				return "cancel", "", nil
			}
			return "cancel", "", nil
		}
		if next != '[' {
			if searchMode {
				return "cancel", "", nil
			}
			return "cancel", "", nil
		}
		arrow, err := reader.ReadByte()
		if err != nil {
			return "", "", err
		}
		switch arrow {
		case 'A':
			return "up", "", nil
		case 'B':
			return "down", "", nil
		default:
			return "", "", nil
		}
	default:
		if searchMode && b >= 0x20 && b != 0x7f {
			return "text", string(b), nil
		}
		return "", "", nil
	}
}

func filteredMenuItems(items []menuItem, query string) ([]menuItem, []int) {
	if strings.TrimSpace(query) == "" {
		indexes := make([]int, len(items))
		for i := range items {
			indexes[i] = i
		}
		return items, indexes
	}

	query = strings.ToLower(query)
	filtered := make([]menuItem, 0, len(items))
	indexes := make([]int, 0, len(items))
	for i, item := range items {
		if strings.Contains(strings.ToLower(item.title), query) || strings.Contains(strings.ToLower(item.details), query) {
			filtered = append(filtered, item)
			indexes = append(indexes, i)
		}
	}
	return filtered, indexes
}

func clearScreen() {
	fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
}

func writeMenuLine(line string) {
	fmt.Fprintf(os.Stdout, "%s\r\n", line)
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

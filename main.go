package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	var configPath string
	var listProfiles bool
	var showExample bool

	flag.StringVar(&configPath, "config", "", "config file or directory; defaults to ~/.lssh.{json,yaml,yml} and ~/.config/lssh")
	flag.BoolVar(&listProfiles, "list", false, "list configured profiles")
	flag.BoolVar(&showExample, "example-config", false, "print an example config file")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] [profile]\n\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Examples:")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s dev\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -config ~/.config/lssh\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -config ./servers.yaml dev\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -list\n\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Interactive transfer tips:")
		fmt.Fprintln(flag.CommandLine.Output(), "  remote download: type `sz <remote-file>` on the server")
		fmt.Fprintln(flag.CommandLine.Output(), "  local upload:   type `~rz` to choose local files, or `~rz <local-file> [more-files...]`")
		fmt.Fprintln(flag.CommandLine.Output(), "  disconnect:     type `~.` at the start of a line")
		fmt.Fprintln(flag.CommandLine.Output())
		flag.PrintDefaults()
	}
	flag.Parse()

	if showExample {
		fmt.Print(exampleConfig())
		return
	}

	sources, err := loadConfigSources(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}

	if listProfiles {
		printProfiles(sources)
		return
	}

	profileName := ""
	if flag.NArg() > 0 {
		profileName = strings.TrimSpace(flag.Arg(0))
	}

	var (
		source       ConfigSource
		profile      SSHProfile
		resolvedName string
	)

	if profileName != "" {
		source, profile, resolvedName, err = selectProfileAcrossSources(sources, profileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "select profile failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		source, profile, resolvedName, err = chooseInteractiveTarget(sources)
		if err != nil {
			msg := selectionErrorMessage(err)
			if msg == "" {
				msg = err.Error()
			}
			if errors.Is(err, errSelectionCanceled) {
				fmt.Fprintln(os.Stderr, msg)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "select profile failed: %s\n", msg)
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "connecting to %s from %s...\n", resolvedName, source.Path)
	if err := RunSSHClient(resolvedName, profile); err != nil {
		fmt.Fprintf(os.Stderr, "ssh session failed: %v\n", err)
		os.Exit(1)
	}
}

func printProfiles(sources []ConfigSource) {
	type row struct {
		source string
		name   string
		target string
	}

	var rows []row
	for _, source := range sources {
		for _, name := range sortedProfileNames(source.Config) {
			profile := source.Config.Profiles[name]
			rows = append(rows, row{
				source: source.Path,
				name:   name,
				target: fmt.Sprintf("%s@%s:%d", profile.User, profile.Host, profile.Port),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].source == rows[j].source {
			return rows[i].name < rows[j].name
		}
		return rows[i].source < rows[j].source
	})

	for _, row := range rows {
		fmt.Printf("%s\t%s\t%s\n", row.source, row.name, row.target)
	}
}

func exampleConfig() string {
	return `default_profile: dev
profiles:
  dev:
    host: 192.168.1.100
    port: 22
    user: root
    private_key_path: ~/.ssh/id_ed25519
    known_hosts: ~/.ssh/known_hosts

  prod:
    host: 10.0.0.10
    user: deploy
    password: your-password
    insecure_ignore_host_key: false
`
}

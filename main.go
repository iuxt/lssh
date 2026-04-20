package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	var configPath string
	var listProfiles bool
	var showExample bool

	flag.StringVar(&configPath, "config", "", "path to config file, default is ~/.tssh.yaml")
	flag.BoolVar(&listProfiles, "list", false, "list configured profiles")
	flag.BoolVar(&showExample, "example-config", false, "print an example config file")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] [profile]\n\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Examples:")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s dev\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -config ./tssh.yaml dev\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -list\n\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Interactive transfer tips:")
		fmt.Fprintln(flag.CommandLine.Output(), "  remote download: type `sz <remote-file>` on the server")
		fmt.Fprintln(flag.CommandLine.Output(), "  local upload:   type `~rz <local-file> [more-files...]` at the start of a line")
		fmt.Fprintln(flag.CommandLine.Output(), "  disconnect:     type `~.` at the start of a line")
		fmt.Fprintln(flag.CommandLine.Output())
		flag.PrintDefaults()
	}
	flag.Parse()

	if showExample {
		fmt.Print(exampleConfig())
		return
	}

	cfg, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
		os.Exit(1)
	}

	if listProfiles {
		for name, profile := range cfg.Profiles {
			fmt.Printf("%s\t%s@%s:%d\n", name, profile.User, profile.Host, profile.Port)
		}
		return
	}

	profileName := ""
	if flag.NArg() > 0 {
		profileName = strings.TrimSpace(flag.Arg(0))
	}

	profile, resolvedName, err := selectProfile(cfg, profileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "select profile failed: %v\n", err)
		os.Exit(1)
	}

	if err := RunSSHClient(resolvedName, profile); err != nil {
		fmt.Fprintf(os.Stderr, "ssh session failed: %v\n", err)
		os.Exit(1)
	}
}

func exampleConfig() string {
	return `{
  "default_profile": "dev",
  "profiles": {
    "dev": {
      "host": "192.168.1.100",
      "port": 22,
      "user": "root",
      "password": "your-password",
      "known_hosts": "~/.ssh/known_hosts",
      "insecure_ignore_host_key": true
    }
  }
}
`
}

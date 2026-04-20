# lssh

`lssh` is a small interactive SSH client for quickly switching between saved hosts, with built-in `rz/sz` file transfer support.

## Features

- Run `lssh` directly to open a terminal UI and choose a config file, then choose a profile.
- Support a single config file or a directory containing multiple config files.
- Support password login and SSH private key login.
- Support `sz` download and `rz` upload during an interactive session.
- `~rz` opens the local file picker on macOS for convenient uploads.

## Install

```bash
go build -o lssh .
```

## Configuration

`lssh` can load config from:

- `~/.lssh.json`
- `~/.lssh.yaml`
- `~/.lssh.yml`
- `~/.config/lssh/` directory

You can also point `-config` to either a single file or a directory.

Supported config file formats:

- `.json`
- `.yaml`
- `.yml`

### Example: SSH key login

```yaml
default_profile: dev
profiles:
  dev:
    host: 192.168.1.100
    port: 22
    user: root
    private_key_path: ~/.ssh/id_ed25519
    known_hosts: ~/.ssh/known_hosts
```

### Example: Password login

```yaml
profiles:
  prod:
    host: 10.0.0.10
    user: deploy
    password: your-password
    known_hosts: ~/.ssh/known_hosts
```

### Example: Encrypted private key

```yaml
profiles:
  bastion:
    host: 10.0.0.5
    user: ops
    private_key_path: ~/.ssh/id_rsa
    private_key_passphrase: your-passphrase
```

### Example: Multiple config files

Put several files under `~/.config/lssh/`:

```text
~/.config/lssh/
  dev.yaml
  prod.yaml
  staging.yaml
```

Then run:

```bash
lssh
```

`lssh` will show a TUI to choose the config file first, then the profile.

## Usage

```bash
lssh
lssh dev
lssh -config ~/.config/lssh
lssh -config ./servers.yaml prod
lssh -list
lssh -example-config
```

If you pass a profile name, `lssh` searches all loaded config files. If the same profile name exists in multiple config files, it will ask you to disambiguate by using the TUI or by passing a narrower `-config` path.

## TUI Controls

- `↑` / `↓` to move
- `j` / `k` to move
- `Enter` to confirm
- `q` or `Ctrl+C` to quit

## File Transfer

Inside the SSH session:

- Remote download: run `sz <remote-file>` on the server
- Local upload with picker: type `~rz`
- Local upload with explicit paths: type `~rz <local-file> [more-files...]`
- Direct remote upload flow: run `rz` on the server, then choose files locally
- Disconnect: type `~.`

## Profile Fields

- `host`: remote host, required
- `port`: SSH port, optional, default `22`
- `user`: SSH username, required
- `password`: password auth
- `private_key_path`: path to a private key file
- `private_key`: inline private key content
- `private_key_passphrase`: passphrase for encrypted private keys
- `known_hosts`: custom known hosts path, default `~/.ssh/known_hosts`
- `insecure_ignore_host_key`: skip host key verification

At least one of `password`, `private_key_path`, or `private_key` must be set.

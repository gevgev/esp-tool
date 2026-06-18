package syncer

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Dial connects to host:port over SSH, authenticating via ssh-agent
// (SSH_AUTH_SOCK) if available, falling back to the private key at keyPath
// if given. Host keys are verified against ~/.ssh/known_hosts — there is no
// option to skip this check, so the user must have connected to the host at
// least once with the system ssh client before using esp-tool sync.
func Dial(host string, port int, user, keyPath string, timeout time.Duration) (*ssh.Client, error) {
	methods, err := authMethods(keyPath)
	if err != nil {
		return nil, err
	}
	hostKeyCB, err := hostKeyCallback()
	if err != nil {
		return nil, err
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            methods,
		HostKeyCallback: hostKeyCB,
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	return client, nil
}

func authMethods(keyPath string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			ag := agent.NewClient(conn)
			methods = append(methods, ssh.PublicKeysCallback(ag.Signers))
		}
	}

	if keyPath != "" {
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("read ssh key %s: %w", keyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse ssh key %s: %w (passphrase-protected keys are not supported — load the key into ssh-agent instead)", keyPath, err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH auth available: start ssh-agent and add a key, or pass --ssh-key")
	}
	return methods, nil
}

func hostKeyCallback() (ssh.HostKeyCallback, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	path := filepath.Join(home, ".ssh", "known_hosts")
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("known_hosts not found at %s — connect once with `ssh` to trust the host, then retry", path)
	}
	return knownhosts.New(path)
}

// ReadFile reads the contents of a remote file over SSH using "cat".
func ReadFile(client *ssh.Client, path string) ([]byte, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new ssh session: %w", err)
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr
	if err := sess.Run("cat " + shellQuote(path)); err != nil {
		return nil, fmt.Errorf("read remote file %s: %w (%s)", path, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// WriteFileAtomic writes data to a remote file over SSH via write-tmp-then-
// rename (cat > tmp && mv tmp path), so the Builder never observes a partial
// write even while it's running.
func WriteFileAtomic(client *ssh.Client, path string, data []byte) error {
	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("new ssh session: %w", err)
	}
	defer sess.Close()

	tmp := path + ".esp-tool-sync.tmp"
	stdin, err := sess.StdinPipe()
	if err != nil {
		return fmt.Errorf("ssh stdin pipe: %w", err)
	}
	var stderr bytes.Buffer
	sess.Stderr = &stderr

	cmd := fmt.Sprintf("cat > %s && mv %s %s", shellQuote(tmp), shellQuote(tmp), shellQuote(path))
	if err := sess.Start(cmd); err != nil {
		return fmt.Errorf("start remote write: %w", err)
	}
	if _, err := stdin.Write(data); err != nil {
		return fmt.Errorf("write remote file %s: %w", path, err)
	}
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("close remote stdin: %w", err)
	}
	if err := sess.Wait(); err != nil {
		return fmt.Errorf("write remote file %s: %w (%s)", path, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// DiscoverRemoteFile searches the standard Home Assistant OS add-on data
// layout for the Device Builder's state file and returns the first match.
// The add-on's data directory is named after a slug
// (/addon_configs/<slug>/data/.device-builder-devices.json) that is opaque
// and instance-specific, so callers should use this instead of asking the
// user to know or guess it.
func DiscoverRemoteFile(client *ssh.Client) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new ssh session: %w", err)
	}
	defer sess.Close()

	var stdout bytes.Buffer
	sess.Stdout = &stdout
	const cmd = `find /addon_configs -maxdepth 3 -name .device-builder-devices.json 2>/dev/null | head -1`
	if err := sess.Run(cmd); err != nil {
		return "", fmt.Errorf("search for device-builder state file: %w", err)
	}

	path := strings.TrimSpace(stdout.String())
	if path == "" {
		return "", fmt.Errorf("could not find .device-builder-devices.json under /addon_configs on the remote host — pass --ssh-remote-file explicitly")
	}
	return path, nil
}

// shellQuote single-quotes s for safe interpolation into a remote shell
// command, escaping any embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

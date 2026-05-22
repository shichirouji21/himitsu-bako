package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"filippo.io/age"
)

const storeName = "himitsu-bako"

const version = "1.0.0"

const defaultClearTimeoutSeconds = 30

// clearAfterFlag is a hidden subcommand used to fork a detached cleaner
// process that wipes the clipboard after a delay if it still contains the
// revealed secret.
const clearAfterFlag = "--clear-after"

type store struct {
	dir           string
	secretsDir    string
	identityFile  string
	recipientFile string
}

type secretRecord struct {
	name string
	path string
}

func main() {
	if len(os.Args) >= 4 && os.Args[1] == clearAfterFlag {
		// Detached helper: read secret bytes from a tmp file, sleep, clear if still present.
		runClearAfter(os.Args[2])
		return
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("himitsu-bako", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	save := flags.Bool("s", false, "save current clipboard as an encrypted secret")
	remove := flags.Bool("r", false, "remove a secret with fzf")
	help := flags.Bool("h", false, "show help")
	versionFlag := flags.Bool("v", false, "print version and exit")
	timeout := flags.Int("timeout", defaultClearTimeoutSeconds, "seconds before clearing the clipboard after reveal (0 to disable)")
	flags.BoolVar(save, "save", false, "save current clipboard as an encrypted secret")
	flags.BoolVar(remove, "remove", false, "remove a secret with fzf")
	flags.BoolVar(help, "help", false, "show help")
	flags.BoolVar(versionFlag, "version", false, "print version and exit")
	flags.Usage = usage

	if err := flags.Parse(args); err != nil {
		return err
	}

	if *help {
		usage()
		return nil
	}
	if *versionFlag {
		fmt.Printf("himitsu-bako %s\n", version)
		return nil
	}
	if *save && *remove {
		return errors.New("choose only one action")
	}
	if *timeout < 0 {
		return errors.New("--timeout must not be negative")
	}

	remaining := flags.Args()
	st, err := defaultStore()
	if err != nil {
		return err
	}

	switch {
	case *save:
		if len(remaining) != 0 {
			return errors.New("-s does not accept a secret name")
		}
		return saveSecret(st)
	case *remove:
		if len(remaining) != 0 {
			return errors.New("-r does not accept a secret name")
		}
		return removeSecret(st)
	case len(remaining) == 0:
		return revealWithFZF(st, *timeout)
	default:
		return revealByName(st, strings.Join(remaining, " "), *timeout)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [-s|-r|name] [--timeout=N]\n", os.Args[0])
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "With no argument, reveal a saved secret with fzf.")
	fmt.Fprintln(os.Stderr, "  name           Reveal the exact secret name without fzf.")
	fmt.Fprintln(os.Stderr, "  -s, --save     Save the current clipboard as an encrypted secret.")
	fmt.Fprintln(os.Stderr, "  -r, --remove   Remove a secret with fzf.")
	fmt.Fprintf(os.Stderr, "  --timeout=N    Clear the clipboard N seconds after reveal (default %d, 0 disables).\n", defaultClearTimeoutSeconds)
	fmt.Fprintln(os.Stderr, "  -v, --version  Print version and exit.")
	fmt.Fprintln(os.Stderr, "  -h, --help     Show this help.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Environment variables:")
	fmt.Fprintln(os.Stderr, "  SECRET_STORE_DIR  Override the encrypted secret store directory.")
}

func defaultStore() (store, error) {
	dir := os.Getenv("SECRET_STORE_DIR")
	if dir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return store{}, fmt.Errorf("could not resolve user config directory: %w", err)
		}
		dir = filepath.Join(configDir, storeName)
	}

	return store{
		dir:           dir,
		secretsDir:    filepath.Join(dir, "secrets"),
		identityFile:  filepath.Join(dir, "identity.txt"),
		recipientFile: filepath.Join(dir, "recipient.txt"),
	}, nil
}

func ensureStoreForWrite(st store) (*age.X25519Identity, error) {
	if err := os.MkdirAll(st.secretsDir, 0o700); err != nil {
		return nil, fmt.Errorf("could not create store directory: %w", err)
	}
	_ = os.Chmod(st.dir, 0o700)
	_ = os.Chmod(st.secretsDir, 0o700)

	identity, err := readIdentity(st.identityFile)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}

		fmt.Fprintf(os.Stderr, "Creating local age identity at %s\n", st.identityFile)
		identity, err = age.GenerateX25519Identity()
		if err != nil {
			return nil, fmt.Errorf("failed to create age identity: %w", err)
		}
		if err := writePrivateFile(st.identityFile, []byte(identity.String()+"\n")); err != nil {
			return nil, fmt.Errorf("failed to write age identity: %w", err)
		}
	}

	recipient := identity.Recipient().String() + "\n"
	if err := writePrivateFile(st.recipientFile, []byte(recipient)); err != nil {
		return nil, fmt.Errorf("failed to write age recipient: %w", err)
	}

	return identity, nil
}

func ensureStoreForRead(st store) (*age.X25519Identity, error) {
	if _, err := os.Stat(st.secretsDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("no secrets saved yet")
		}
		return nil, fmt.Errorf("could not read secrets directory: %w", err)
	}

	paths, err := secretFiles(st)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errors.New("no secrets saved yet")
	}

	identity, err := readIdentity(st.identityFile)
	if err != nil {
		return nil, err
	}
	return identity, nil
}

func readIdentity(path string) (*age.X25519Identity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		identity, err := age.ParseX25519Identity(line)
		if err != nil {
			return nil, fmt.Errorf("identity file %q contains an invalid age identity: %w", path, err)
		}
		return identity, nil
	}

	return nil, fmt.Errorf("identity file %q does not contain an age identity", path)
}

func saveSecret(st store) error {
	identity, err := ensureStoreForWrite(st)
	if err != nil {
		return err
	}

	secretName, err := promptSecretName()
	if err != nil {
		return err
	}

	clipboard, err := clipboardRead()
	if err != nil {
		return err
	}
	if len(clipboard) == 0 {
		return errors.New("clipboard is empty")
	}

	payload := append([]byte(secretName+"\n"), clipboard...)
	matches, err := findSecretFilesByName(st, identity, secretName)
	if err != nil {
		return err
	}

	target := ""
	if len(matches) > 0 {
		target = matches[0]
	} else {
		target, err = newSecretPath(st.secretsDir)
		if err != nil {
			return err
		}
	}

	if err := encryptPayloadToFile(payload, target, identity.Recipient()); err != nil {
		return err
	}
	if len(matches) > 1 {
		for _, duplicate := range matches[1:] {
			_ = os.Remove(duplicate)
		}
	}

	fmt.Fprintf(os.Stderr, "Saved %q.\n", secretName)
	return nil
}

func revealWithFZF(st store, timeoutSeconds int) error {
	identity, err := ensureStoreForRead(st)
	if err != nil {
		return err
	}
	record, err := selectSecretRecord(st, identity, "secret> ")
	if err != nil {
		return err
	}
	return copySecretToClipboard(identity, record, timeoutSeconds)
}

func revealByName(st store, name string, timeoutSeconds int) error {
	identity, err := ensureStoreForRead(st)
	if err != nil {
		return err
	}
	if err := validateSecretName(name); err != nil {
		return err
	}

	matches, err := findSecretFilesByName(st, identity, name)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("secret %q was not found", name)
	}

	return copySecretToClipboard(identity, secretRecord{name: name, path: matches[0]}, timeoutSeconds)
}

func removeSecret(st store) error {
	identity, err := ensureStoreForRead(st)
	if err != nil {
		return err
	}
	record, err := selectSecretRecord(st, identity, "remove secret> ")
	if err != nil {
		return err
	}
	if err := os.Remove(record.path); err != nil {
		return fmt.Errorf("failed to remove %q: %w", record.name, err)
	}
	fmt.Fprintf(os.Stderr, "Removed %q.\n", record.name)
	return nil
}

func promptSecretName() (string, error) {
	fmt.Fprint(os.Stderr, "Secret name: ")
	name, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("failed to read secret name: %w", err)
	}
	name = strings.TrimRight(name, "\r\n")
	if err := validateSecretName(name); err != nil {
		return "", err
	}
	return name, nil
}

func validateSecretName(name string) error {
	if name == "" {
		return errors.New("secret name must not be empty")
	}
	if strings.ContainsAny(name, "\t\r\n") {
		return errors.New("secret name must not contain tabs or newlines")
	}
	return nil
}

func findSecretFilesByName(st store, identity age.Identity, name string) ([]string, error) {
	paths, err := secretFiles(st)
	if err != nil {
		return nil, err
	}

	matches := make([]string, 0)
	for _, path := range paths {
		recordName, err := decryptSecretName(identity, path)
		if err != nil {
			continue
		}
		if recordName == name {
			matches = append(matches, path)
		}
	}
	return matches, nil
}

func secretFiles(st store) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(st.secretsDir, "*.age"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func decryptSecretName(identity age.Identity, path string) (string, error) {
	payload, err := decryptFile(identity, path)
	if err != nil {
		return "", err
	}
	name, _, ok := bytes.Cut(payload, []byte("\n"))
	if !ok || len(name) == 0 {
		return "", errors.New("secret payload has no name line")
	}
	return string(name), nil
}

func listSecretRecords(st store, identity age.Identity) ([]secretRecord, error) {
	paths, err := secretFiles(st)
	if err != nil {
		return nil, err
	}

	records := make([]secretRecord, 0, len(paths))
	for _, path := range paths {
		name, err := decryptSecretName(identity, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping unreadable secret file %q\n", path)
			continue
		}
		records = append(records, secretRecord{name: name, path: path})
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].name < records[j].name
	})
	return records, nil
}

func selectSecretRecord(st store, identity age.Identity, prompt string) (secretRecord, error) {
	if _, err := exec.LookPath("fzf"); err != nil {
		return secretRecord{}, errors.New("fzf is not installed. Install fzf, for example: brew install fzf, apt install fzf, or dnf install fzf")
	}

	records, err := listSecretRecords(st, identity)
	if err != nil {
		return secretRecord{}, err
	}
	if len(records) == 0 {
		return secretRecord{}, errors.New("no readable secrets found")
	}

	var input strings.Builder
	for _, record := range records {
		input.WriteString(record.name)
		input.WriteByte('\t')
		input.WriteString(record.path)
		input.WriteByte('\n')
	}

	cmd := exec.Command("fzf", "--delimiter=\t", "--with-nth=1", "--no-multi", "--prompt="+prompt)
	cmd.Stdin = strings.NewReader(input.String())
	cmd.Stderr = os.Stderr
	selected, err := cmd.Output()
	if err != nil {
		return secretRecord{}, errors.New("no secret selected")
	}

	line := strings.TrimRight(string(selected), "\r\n")
	name, path, ok := strings.Cut(line, "\t")
	if !ok || name == "" || path == "" {
		return secretRecord{}, errors.New("invalid secret selection")
	}
	return secretRecord{name: name, path: path}, nil
}

func scheduleClipboardClear(secret []byte, timeoutSeconds int) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp("", "himitsu-bako-clear-*")
	if err != nil {
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if _, err := tmp.Write(secret); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}

	cmd := exec.Command(exe, clearAfterFlag, strconv.Itoa(timeoutSeconds), tmp.Name())
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	detachCmd(cmd)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := cmd.Process.Release(); err != nil {
		return err
	}
	return nil
}

func runClearAfter(arg string) {
	seconds, err := strconv.Atoi(arg)
	if err != nil || seconds <= 0 {
		return
	}
	if len(os.Args) < 4 {
		return
	}
	secretFile := os.Args[3]
	defer os.Remove(secretFile)

	secret, err := os.ReadFile(secretFile)
	if err != nil || len(secret) == 0 {
		return
	}
	time.Sleep(time.Duration(seconds) * time.Second)
	current, err := clipboardRead()
	if err != nil {
		return
	}
	if !bytes.Equal(current, secret) {
		return
	}
	_ = clipboardWrite(nil)
}

func copySecretToClipboard(identity age.Identity, record secretRecord, timeoutSeconds int) error {
	payload, err := decryptFile(identity, record.path)
	if err != nil {
		return fmt.Errorf("failed to decrypt %q: %w", record.name, err)
	}
	_, secret, ok := bytes.Cut(payload, []byte("\n"))
	if !ok {
		return fmt.Errorf("secret %q has invalid payload", record.name)
	}
	if err := clipboardWrite(secret); err != nil {
		return err
	}
	if timeoutSeconds > 0 {
		if err := scheduleClipboardClear(secret, timeoutSeconds); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not schedule clipboard clear: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Clipboard will be cleared in %ds.\n", timeoutSeconds)
		}
	}
	return nil
}

func decryptFile(identity age.Identity, path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader, err := age.Decrypt(file, identity)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(reader)
}

func encryptPayloadToFile(payload []byte, target string, recipient age.Recipient) error {
	tmp, err := newTempSecretPath(filepath.Dir(target))
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writer, err := age.Encrypt(file, recipient)
	if err != nil {
		file.Close()
		return err
	}
	if _, err := writer.Write(payload); err != nil {
		writer.Close()
		file.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	cleanup = false
	_ = os.Chmod(target, 0o600)
	return nil
}

func newSecretPath(dir string) (string, error) {
	return uniquePath(dir, "secret.", ".age")
}

func newTempSecretPath(dir string) (string, error) {
	return uniquePath(dir, ".secret.", ".age")
}

func uniquePath(dir, prefix, suffix string) (string, error) {
	for range 100 {
		name, err := randomToken(8)
		if err != nil {
			return "", err
		}
		path := filepath.Join(dir, prefix+name+suffix)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return path, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", errors.New("could not allocate unique secret file")
}

func randomToken(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	for i, value := range raw {
		raw[i] = alphabet[int(value)%len(alphabet)]
	}
	return string(raw), nil
}

func writePrivateFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

func clipboardRead() ([]byte, error) {
	switch {
	case commandExists("pbpaste"):
		return commandOutput("pbpaste")
	case commandExists("wl-paste"):
		return commandOutput("wl-paste", "--no-newline")
	case commandExists("xclip"):
		return commandOutput("xclip", "-selection", "clipboard", "-out")
	case commandExists("xsel"):
		return commandOutput("xsel", "--clipboard", "--output")
	case commandExists("powershell.exe"):
		return commandOutput("powershell.exe", "-NoProfile", "-Command", "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Get-Clipboard -Raw")
	default:
		return nil, errors.New("no clipboard reader found. Install pbpaste, wl-paste, xclip, xsel, or run from Windows with powershell.exe available")
	}
}

func clipboardWrite(data []byte) error {
	switch {
	case commandExists("pbcopy"):
		return commandInput(data, "pbcopy")
	case commandExists("wl-copy"):
		return commandInput(data, "wl-copy")
	case commandExists("xclip"):
		return commandInput(data, "xclip", "-selection", "clipboard", "-in")
	case commandExists("xsel"):
		return commandInput(data, "xsel", "--clipboard", "--input")
	case commandExists("powershell.exe"):
		return commandInput(data, "powershell.exe", "-NoProfile", "-Command", "[Console]::InputEncoding = [System.Text.Encoding]::UTF8; $text = [Console]::In.ReadToEnd(); Set-Clipboard -Value $text")
	case commandExists("clip.exe"):
		return commandInput(data, "clip.exe")
	default:
		return errors.New("no clipboard writer found. Install pbcopy, wl-copy, xclip, xsel, or run from Windows with powershell.exe/clip.exe available")
	}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func commandOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

func commandInput(input []byte, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

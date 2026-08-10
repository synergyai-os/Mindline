package localservice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/synergyai-os/Mindline/internal/privateio"
)

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("resolve LaunchAgents directory")
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"), nil
}

func writeLaunchAgent(path, binaryPath, configPath, runtimeRoot string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("prepare LaunchAgents directory")
	}
	stdoutPath := filepath.Join(runtimeRoot, "service.stdout.log")
	stderrPath := filepath.Join(runtimeRoot, "service.stderr.log")
	for _, logPath := range []string{stdoutPath, stderrPath} {
		if err := preparePrivateLog(logPath); err != nil {
			return errors.New("prepare local agent logs")
		}
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>` + xmlEscape(launchAgentLabel) + `</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + xmlEscape(binaryPath) + `</string>
    <string>agent</string>
    <string>service-run</string>
    <string>--config</string>
    <string>` + xmlEscape(configPath) + `</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>5</integer>
  <key>StandardOutPath</key><string>` + xmlEscape(stdoutPath) + `</string>
  <key>StandardErrorPath</key><string>` + xmlEscape(stderrPath) + `</string>
</dict>
</plist>
`
	if err := writePrivateFilePreservingParent(path, []byte(xml)); err != nil {
		return errors.New("write LaunchAgent")
	}
	return nil
}

func preparePrivateLog(path string) error {
	file, err := os.OpenFile(
		path, os.O_CREATE|os.O_APPEND|os.O_WRONLY|syscall.O_NOFOLLOW, privateio.FileMode,
	)
	if err != nil {
		return err
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		file.Close()
		return errors.New("private log is not a regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		file.Close()
		return errors.New("private log is not owner controlled")
	}
	if err := file.Chmod(privateio.FileMode); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func writePrivateFilePreservingParent(path string, data []byte) error {
	info, err := os.Lstat(path)
	if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return errors.New("private install file is unsafe")
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|syscall.O_NOFOLLOW, privateio.FileMode)
	if err != nil {
		return err
	}
	if err := file.Chmod(privateio.FileMode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func xmlEscape(value string) string {
	var escaped bytes.Buffer
	for _, character := range value {
		switch character {
		case '&':
			escaped.WriteString("&amp;")
		case '<':
			escaped.WriteString("&lt;")
		case '>':
			escaped.WriteString("&gt;")
		case '"':
			escaped.WriteString("&quot;")
		case '\'':
			escaped.WriteString("&apos;")
		default:
			escaped.WriteRune(character)
		}
	}
	return escaped.String()
}

func restartLaunchAgent(plistPath string) error {
	return restartLaunchAgentContext(context.Background(), plistPath)
}

func restartLaunchAgentContext(ctx context.Context, plistPath string) error {
	domain := "gui/" + strconv.Itoa(os.Getuid())
	serviceTarget := domain + "/" + launchAgentLabel
	loaded, err := launchAgentLoadedContext(ctx, serviceTarget)
	if err != nil {
		return err
	}
	if loaded {
		command := exec.CommandContext(ctx, "launchctl", "kickstart", "-k", serviceTarget)
		if output, err := command.CombinedOutput(); err != nil {
			if ctx.Err() != nil {
				return errors.New("restart local agent service deadline exceeded")
			}
			return fmt.Errorf("kickstart local agent service: %s", safeCommandError(output))
		}
		return nil
	}

	command := exec.CommandContext(ctx, "launchctl", "bootstrap", domain, plistPath)
	if output, err := command.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return errors.New("restart local agent service deadline exceeded")
		}
		return fmt.Errorf("start local agent service: %s", safeCommandError(output))
	}
	command = exec.CommandContext(ctx, "launchctl", "kickstart", "-k", serviceTarget)
	if output, err := command.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return errors.New("restart local agent service deadline exceeded")
		}
		return fmt.Errorf("kickstart local agent service: %s", safeCommandError(output))
	}
	return nil
}

func stopLaunchAgent(_ string) error {
	domain := "gui/" + strconv.Itoa(os.Getuid())
	serviceTarget := domain + "/" + launchAgentLabel
	loaded, err := launchAgentLoaded(serviceTarget)
	if err != nil || !loaded {
		return err
	}
	command := exec.Command("launchctl", "bootout", serviceTarget)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("stop local agent service: %s", safeCommandError(output))
	}
	return nil
}

func launchAgentRunning(_ string) (bool, error) {
	domain := "gui/" + strconv.Itoa(os.Getuid())
	return launchAgentLoaded(domain + "/" + launchAgentLabel)
}

func launchAgentLoaded(serviceTarget string) (bool, error) {
	return launchAgentLoadedContext(context.Background(), serviceTarget)
}

func launchAgentLoadedContext(ctx context.Context, serviceTarget string) (bool, error) {
	command := exec.CommandContext(ctx, "launchctl", "print", serviceTarget)
	output, err := command.CombinedOutput()
	if err == nil {
		return true, nil
	}
	if ctx.Err() != nil {
		return false, errors.New("inspect local agent service deadline exceeded")
	}
	message := strings.ToLower(string(output))
	if strings.Contains(message, "could not find") ||
		strings.Contains(message, "not found") ||
		strings.Contains(message, "no such process") {
		return false, nil
	}
	return false, fmt.Errorf("inspect local agent service: %s", safeCommandError(output))
}

func safeCommandError(output []byte) string {
	value := strings.TrimSpace(string(output))
	if value == "" {
		return "launchctl failed"
	}
	if len([]rune(value)) > 500 {
		return string([]rune(value)[:500])
	}
	return value
}

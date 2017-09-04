package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/nlopes/slack"
)

type ChannelType struct {
	Recipients []string `json:recipients`
}
type configType struct {
	Channels ChannelType
}

// BufferMaxLines store and send only last numer of stdout and stderr lines
var BufferMaxLines = 15

func prepareCommand() *exec.Cmd {
	if len(os.Args) > 1 {
		return exec.Command(os.Args[1], os.Args[2:]...)
	}
	return nil
}

func ignoreEmpty(prefix string, buffer []string) string {
	if len(buffer) > 0 {
		return prefix + strings.Join(buffer, "\n")
	} else {
		return ""
	}
}

func formatMessage(stdoutBuffer []string, stderrBuffer []string) string {
	return strings.Join([]string{
		"```",
		"CMD: " + strings.Join(os.Args[1:], " "),
		ignoreEmpty("STDERR:\n", stderrBuffer),
		ignoreEmpty("STDOUT:\n", stdoutBuffer),
		"```"}, "\n")
}

func readStd(stdout io.ReadCloser, output *os.File) (buffer []string) {
	in := bufio.NewScanner(stdout)

	for in.Scan() {
		buffer = append(buffer, in.Text())

		if len(buffer) > BufferMaxLines {
			buffer = append(buffer[:0], buffer[1:]...)
		}

		fmt.Fprintln(output, in.Text())
	}

	if err := in.Err(); err != nil {
		fmt.Println("error: %s", err)
	}

	return buffer
}

func runCommand(cmd *exec.Cmd) (stdoutBuffer []string, stderrBuffer []string, exitCode int) {
	stdout, outerr := cmd.StdoutPipe()
	stderr, errerr := cmd.StderrPipe()

	if outerr != nil || errerr != nil {
		return stdoutBuffer, stderrBuffer, -1
	}

	if err := cmd.Start(); err != nil {
		return stdoutBuffer, stderrBuffer, -1
	}

	stdoutBuffer = readStd(stdout, os.Stdout)
	stderrBuffer = readStd(stderr, os.Stderr)

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				exitCode = status.ExitStatus()
			}
		}
	}

	return stdoutBuffer, stderrBuffer, exitCode
}

func Configuration() (config map[string]ChannelType) {
	raw, _ := ioutil.ReadFile(".docker-boom.json")
	json.Unmarshal(raw, &config)
	return config
}

func sendLogs(stdoutBuffer []string, stderrBuffer []string, exitCode int, conf map[string]ChannelType) {
	if exitCode == 0 || len(stdoutBuffer) < 1 && len(stderrBuffer) < 1 {
		return
	}

	if _, ok := conf["slack"]; !ok {
		return
	}

	slackToken := os.Getenv("SLACK_TOKEN")

	if slackToken == "" {
		return
	}

	api := slack.New(slackToken)
	params := slack.PostMessageParameters{}

	for _, re := range conf["slack"].Recipients {
		api.PostMessage(re, formatMessage(stdoutBuffer, stderrBuffer), params)
	}
}

func main() {
	conf := Configuration()

	if cmd := prepareCommand(); cmd != nil {
		stdoutBuffer, stderrBuffer, exitCode := runCommand(cmd)
		sendLogs(stdoutBuffer, stderrBuffer, exitCode, conf)
		os.Exit(exitCode)
	}
}

// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package utils

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	TemplateElapsedTime   string = "%{elapsed_time}"
	TemplateFileCount     string = "%{file_count}"
	TemplateTotalFileSize string = "%{total_file_size}"
	TemplateFilePath      string = "%{file_path}"
	TemplateFileSize      string = "%{file_size}"
)

type Command struct {
	onCompleteCommand         func(fileCount int, totalFileSize int64, elapsedTime string)
	onEachFileCompleteCommand func(filePath string, fileSize int64)
	commandTimeout            time.Duration

	commandResultWaitGroup sync.WaitGroup
	commandChannel         chan string
	waitStopChannel        chan struct{}
}

func NewCommand(onCompleteCommand, onEachFileCompleteCommand string, commandTimeout time.Duration) *Command {
	c := &Command{
		commandTimeout:  commandTimeout,
		commandChannel:  make(chan string, 1),
		waitStopChannel: make(chan struct{}),
	}

	if onCompleteCommand == "" {
		c.onCompleteCommand = func(fileCount int, totalFileSize int64, elapsedTime string) {}
	} else {
		c.onCompleteCommand = func(fileCount int, totalFileSize int64, elapsedTime string) {
			c.runOnComplete(onCompleteCommand, fileCount, totalFileSize, elapsedTime)
		}
	}

	if onEachFileCompleteCommand == "" {
		c.onEachFileCompleteCommand = func(filePath string, fileSize int64) {}
	} else {
		c.onEachFileCompleteCommand = func(filePath string, fileSize int64) {
			c.runOnEachFileComplete(onEachFileCompleteCommand, filePath, fileSize)
		}
	}

	return c
}

func (c *Command) run(command string) {
	defer c.commandResultWaitGroup.Done()

	cmd := exec.Command("bash", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	// Start the command first to ensure cmd.Process is initialized
	if err := cmd.Start(); err != nil {
		ErrorPrintf("Failed to start the command.: (%s) %s", command, err.Error())
		return
	}

	resultChan := make(chan error, 1)
	go func() {
		defer close(resultChan)
		resultChan <- cmd.Wait()
	}()

	select {
	case <-time.After(c.commandTimeout):
		// Terminates all processes belonging to the group.
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		ErrorPrintf("Failed to execute the command.: (%s) timeout occurred while running the command after %v", command, c.commandTimeout)
	case err := <-resultChan:
		if err != nil {
			ErrorPrintf("Failed to execute the command.: (%s) error running command (%s)", command, err.Error())
		}
	}
}

func (c *Command) runOnComplete(command string, fileCount int, totalFileSize int64, elapsedTime string) {
	command = strings.Replace(command, TemplateFileCount, strconv.Itoa(fileCount), -1)
	command = strings.Replace(command, TemplateTotalFileSize, strconv.FormatInt(totalFileSize, 10), -1)
	command = strings.Replace(command, TemplateElapsedTime, elapsedTime, -1)

	c.commandResultWaitGroup.Add(1)
	c.commandChannel <- command
}

func (c *Command) runOnEachFileComplete(command string, filePath string, fileSize int64) {
	command = strings.Replace(command, TemplateFilePath, filePath, -1)
	command = strings.Replace(command, TemplateFileSize, strconv.FormatInt(fileSize, 10), -1)

	c.commandResultWaitGroup.Add(1)
	c.commandChannel <- command
}

func (c *Command) Run() {
	go func() {
		defer close(c.waitStopChannel)

		// Process commands until the channel is closed
		// This ensures all queued commands are executed even after Stop() is called
		for command := range c.commandChannel {
			go c.run(command)
		}
	}()
}

func (c *Command) Stop() {
	// Close the command channel first to prevent new commands from being queued
	// The Run() goroutine will continue processing all remaining commands in the channel
	close(c.commandChannel)

	// Wait for the Run() goroutine to finish processing all queued commands
	<-c.waitStopChannel

	// Wait for all commands to complete execution
	c.commandResultWaitGroup.Wait()
}

func (c *Command) RunOnComplete(fileCount int, totalFileSize int64, elapsedTime string) {
	c.onCompleteCommand(fileCount, totalFileSize, elapsedTime)
}

func (c *Command) RunOnEachFileComplete(filePath string, fileSize int64) {
	c.onEachFileCompleteCommand(filePath, fileSize)
}

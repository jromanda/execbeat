package beat

import (
	"bytes"
	"github.com/christiangalsterer/execbeat/config"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/robfig/cron"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type Executor struct {
	execbeat     *Execbeat
	config       config.ExecConfig
	schedule     string
	documentType string
}

func NewExecutor(execbeat *Execbeat, config config.ExecConfig) *Executor {
	executor := &Executor{
		execbeat: execbeat,
		config:   config,
	}

	return executor
}

func (e *Executor) Run() {

	// setup default config
	e.documentType = config.DefaultDocumentType
	e.schedule = config.DefaultSchedule

	// setup document type
	if e.config.DocumentType != "" {
		e.documentType = e.config.DocumentType
	}

	// setup cron schedule
	if e.config.Schedule != "" {
		logp.Debug("Execbeat", "Use schedule: [%w]", e.config.Schedule)
		e.schedule = e.config.Schedule
	}

	cron := cron.New()
	cron.AddFunc(e.schedule, func() { e.runOneTime() })
	cron.Start()
}

func (e *Executor) runOneTime() error {
	var cmd *exec.Cmd
	var cmdArgs []string
	var err error
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var waitStatus syscall.WaitStatus
	var exitCode int = 0

	cmdName := strings.TrimSpace(e.config.Command)

	args := strings.TrimSpace(e.config.Args)
	if len(args) > 0 {
		cmdArgs = strings.Split(args, " ")
	}

	// execute command
	now := time.Now()

	if len(cmdArgs) > 0 {
		logp.Debug("Execbeat", "Executing command: [%v] with args [%v]", cmdName, cmdArgs)
		cmd = exec.Command(cmdName, cmdArgs...)
	} else {
		logp.Debug("Execbeat", "Executing command: [%v]", cmdName)
		cmd = exec.Command(cmdName)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Start()
	if err != nil {
		logp.Err("An error occured while executing command: %v", err)
		exitCode = 127
	}

	err = cmd.Wait()
	if err != nil {
		logp.Err("An error occured while executing command: %v", err)
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus = exitError.Sys().(syscall.WaitStatus)
			exitCode = waitStatus.ExitStatus()
		}
	}

	logp.Info("Execbeat", "Executing command: [%v]", stdout.String())

	e.publishOutput(cmdName, exitCode, now, stdout.String(), stderr.String())

	return nil
}

func (e *Executor) publishOutput(cmdName string, exitCode int, now time.Time, stdout string, stderr string) {

	if !e.config.SplitLines {
		// original behavior, publish a single event with both stdout + stderr
		commandEvent := Exec{
			Command:  cmdName,
			StdOut:   stdout,
			StdErr:   stderr,
			ExitCode: exitCode,
		}

		event := ExecEvent{
			ReadTime:     now,
			DocumentType: e.documentType,
			Fields:       e.config.Fields,
			Exec:         commandEvent,
		}

		e.execbeat.client.PublishEvent(event.ToMapStr())

	} else {
		// publish separate events for stderr and each stdout line
		if len(stderr)>0 {
			commandEvent := Exec{
				Command:  cmdName,
				StdErr:   stderr,
				ExitCode: exitCode,
			}

			event := ExecEvent{
				ReadTime:     now,
				DocumentType: e.documentType,
				Fields:       e.config.Fields,
				Exec:         commandEvent,
			}

			e.execbeat.client.PublishEvent(event.ToMapStr())
		}

		lo := 0
		for _, s := range strings.Split(stdout, "\n") {

			if len(s) > 0 {
				commandEvent := Exec{
					Command:    cmdName,
					StdOut:     s,
					LineOffset: lo,
					ExitCode:   exitCode,
				}
		
				event := ExecEvent{
					ReadTime:     now,
					DocumentType: e.documentType,
					Fields:       e.config.Fields,
					Exec:         commandEvent,
				}
		
				e.execbeat.client.PublishEvent(event.ToMapStr())

				lo++
			}
		}

	}

}

func (e *Executor) Stop() {
}

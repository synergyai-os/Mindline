package assurance

import (
	"os/exec"
	"time"
)

// ConfigureBoundedProcess applies the assurance runner's cross-platform child
// process cancellation policy to another proof executor. It prevents a timed
// out proof command from leaving descendant test or build processes behind.
func ConfigureBoundedProcess(command *exec.Cmd) {
	configureProcessGroup(command)
	command.Cancel = func() error { return killProcessGroup(command) }
	command.WaitDelay = 5 * time.Second
}

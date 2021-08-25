package shell

import (
	"github.com/oliver256/go-engine/src/loggo"
	"testing"
)

func Test0001(t *testing.T) {
	loggo.Info("start 1")
	RunExeTimeout("sleep", false, 2, "5")
	loggo.Info("end 1")
}

func Test0002(t *testing.T) {
	loggo.Info("start 2")
	RunExe("sleep", false, "2")
	loggo.Info("end 2")
}

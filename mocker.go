package commandmocker

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"
)

var source = `#!/bin/bash -e

echo=$(which echo)
output=$(cat <<EOF
{{.output}}
EOF
)
erroutput=$(cat <<EOF
{{.erroutput}}
EOF
)
dirname=$(dirname ${0})

$echo -n "${output}" | tee -a ${dirname}/.out
$echo -n "${erroutput}" >&2 | tee -a ${dirname}/.err

for i in "$@"
do
	$echo -- "$i" | sed -e 's/-- //' >> ${dirname}/.params
done
touch ${dirname}/.ran
env >> ${dirname}/.envs
exit {{.status}}
`
var running map[string]string
var runningMutex sync.RWMutex
var pathMutex sync.Mutex

func init() {
	running = map[string]string{}
}

func add(name, stdout, stderr string, status int) (string, error) {
	for {
		runningMutex.RLock()
		_, ok := running[name]
		runningMutex.RUnlock()
		if !ok {
			break
		}
		time.Sleep(1)
	}
	runningMutex.Lock()
	var buf [8]byte
	rand.Read(buf[:])
	tempdir := path.Join(os.TempDir(), fmt.Sprintf("commandmocker-%x", buf))
	_, err := os.Stat(tempdir)
	for !os.IsNotExist(err) {
		rand.Read(buf[:])
		tempdir = path.Join(os.TempDir(), fmt.Sprintf("commandmocker-%x", buf))
		_, err = os.Stat(tempdir)
	}
	running[name] = tempdir
	runningMutex.Unlock()
	err = os.MkdirAll(tempdir, 0777)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path.Join(tempdir, name), syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC, 0755)
	if err != nil {
		return "", err
	}
	defer f.Close()
	t, err := template.New(name).Parse(source)
	if err != nil {
		return "", err
	}
	param := map[string]interface{}{
		"output":    stdout,
		"erroutput": stderr,
		"status":    status,
	}
	err = t.Execute(f, param)
	if err != nil {
		return "", err
	}
	pathMutex.Lock()
	path := os.Getenv("PATH")
	path = tempdir + ":" + path
	err = os.Setenv("PATH", path)
	pathMutex.Unlock()
	return tempdir, nil
}

// Add creates a temporary directory containing an executable file named "name"
// that prints "output" when executed. It also adds the temporary directory to
// the first position of $PATH.
//
// It returns the temporary directory path (for future removing, using the
// Remove function) and an error if any happen.
func Add(name, output string) (string, error) {
	return add(name, output, "", 0)
}

// AddStderr works like Add, but it allow callers to specify the output for
// both the stdout and stderr streams.
func AddStderr(name, stdout, stderr string) (string, error) {
	return add(name, stdout, stderr, 0)
}

// Error works like Add, but the created executable returns a non-zero status
// code (an error). The returned status code will be the value provided by
// status.
func Error(name, output string, status int) (string, error) {
	return add(name, "", output, status)
}

// Ran indicates whether the mocked executable was called or not.
//
// It just checks if the given tempdir contains a .ran file.
func Ran(tempdir string) bool {
	p := path.Join(tempdir, ".ran")
	_, err := os.Stat(p)
	return err == nil || !os.IsNotExist(err)
}

// Output returns the output generated by the previously added command
// execution.
func Output(tempdir string) string {
	p := path.Join(tempdir, ".out")
	b, err := ioutil.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// Envs returns the environment variables available to the previously added
// command execution.
func Envs(tempdir string) string {
	envs := path.Join(tempdir, ".envs")
	b, err := ioutil.ReadFile(envs)
	if err != nil {
		return ""
	}
	return string(b)
}

// Parameters returns a slice containing all positional parameters given to the
// command mocked in tempdir in its last execution.
func Parameters(tempdir string) []string {
	p := path.Join(tempdir, ".params")
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	var params []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		params = append(params, scanner.Text())
	}
	return params
}

// Remove removes the tempdir from $PATH and from file system.
//
// This function is intended only to undo what Add does. It returns error if
// the given tempdir is not a temporary directory.
func Remove(tempdir string) error {
	defer func() {
		var k string
		runningMutex.Lock()
		for key, value := range running {
			if value == tempdir {
				k = key
			}
		}
		delete(running, k)
		runningMutex.Unlock()
	}()
	if !strings.HasPrefix(tempdir, os.TempDir()) {
		return errors.New("Remove can only remove temporary directories, tryied to remove " + tempdir)
	}
	path := os.Getenv("PATH")
	index := strings.Index(path, tempdir)
	if index < 0 {
		return fmt.Errorf("%q is not in $PATH", tempdir)
	}
	path = path[:index] + path[index+len(tempdir)+1:]
	err := os.Setenv("PATH", path)
	if err != nil {
		return err
	}
	return os.RemoveAll(tempdir)
}

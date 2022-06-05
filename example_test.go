package commandmocker_test

import (
	"fmt"
	"github.com/thifnmi/mypaas/commandmocker"
	"os/exec"
)

// This example demonstrates the mocking of the SSH command.
func ExampleAdd() {
	msg := "HELP!"
	path, err := commandmocker.Error("ssh", msg, 1)
	if err != nil {
		panic(err)
	}
	defer commandmocker.Remove(path)
	cmd := exec.Command("ssh", "-l", "root", "127.0.0.1")
	out, err := cmd.CombinedOutput()
	fmt.Println(err)
	fmt.Printf("%s\n", out)
	fmt.Println(commandmocker.Parameters(path))
	fmt.Println(commandmocker.Ran(path))
	// Output:
	// exit status 1
	// HELP!
	// [-l root 127.0.0.1]
	// true
}

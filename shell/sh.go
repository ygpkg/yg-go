package shell

import "os/exec"

// ShExec sh -c cmd
func ShExec(cmdstr string) (string, error) {
	cmd := exec.Command("sh", "-c", cmdstr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

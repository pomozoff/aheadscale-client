package services

import "os/exec"

// Bundles have already been exported into this user's Temp before reveal.
func revealFile(path string) error {
	return exec.Command("explorer.exe", "/select,"+path).Start()
}

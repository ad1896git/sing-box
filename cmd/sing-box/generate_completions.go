//go:build generate && generate_completions

package boxmain

// GenerateCompletions writes the shell completion files to release/completions.
func GenerateCompletions() error {
	err := mainCommand.GenBashCompletionFile("release/completions/sing-box.bash")
	if err != nil {
		return err
	}
	err = mainCommand.GenFishCompletionFile("release/completions/sing-box.fish", true)
	if err != nil {
		return err
	}
	err = mainCommand.GenZshCompletionFile("release/completions/sing-box.zsh")
	if err != nil {
		return err
	}
	return nil
}

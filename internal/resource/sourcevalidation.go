package resource

import (
	"fmt"
	"regexp"
	"strings"
)

// maxDockerfileBytes bounds an inline Dockerfile. A multi-megabyte literal embedded in a
// config file would otherwise be parsed, validated and shipped to the API verbatim; the
// limit caps that memory cost.
const maxDockerfileBytes = 1 << 20 // 1 MiB

// gitBranchPattern restricts a branch name to the characters a normal git ref uses. It
// rejects whitespace and option-injection payloads such as "--upload-pack=evil" before the
// value reaches a remote clone, where a leaked option could change what the clone executes.
var gitBranchPattern = regexp.MustCompile(`^[a-zA-Z0-9._/-]{1,255}$`)

// gitRepositorySchemes is the allowlist of URL prefixes a git_repository may use. Anything
// else (file://, ftp://, javascript:, an unknown scheme) is refused rather than passed
// through to git.
var gitRepositorySchemes = []string{"https://", "http://", "git@"}

// validate checks a git source's required fields and their formats.
func (src SourceSpec) validate() error {
	if src.GitRepository == "" || src.GitBranch == "" || src.PortsExposes == "" {
		return fmt.Errorf("spec.source: git_repository, git_branch and ports_exposes are all required")
	}
	if err := validateGitRepository(src.GitRepository); err != nil {
		return err
	}
	return validateGitBranch(src.GitBranch)
}

func validateDockerfile(content string) error {
	if len(content) > maxDockerfileBytes {
		return fmt.Errorf("spec.dockerfile: content exceeds 1 MB limit (%d bytes)", len(content))
	}
	return nil
}

func validateGitBranch(branch string) error {
	if !gitBranchPattern.MatchString(branch) {
		return fmt.Errorf("spec.source.git_branch: invalid branch name format %q (allowed: letters, digits, dot, underscore, slash, hyphen)", branch)
	}
	return nil
}

func validateGitRepository(repo string) error {
	for _, scheme := range gitRepositorySchemes {
		if strings.HasPrefix(repo, scheme) {
			return nil
		}
	}
	return fmt.Errorf("spec.source.git_repository: unsupported URL scheme in %q (allowed schemes: https:// http:// git@)", repo)
}

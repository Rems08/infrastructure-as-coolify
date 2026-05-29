package coolify

import "fmt"

// applicationCreateEndpoint selects the Coolify v4 create endpoint (relative to /api/v1)
// and the build_pack the request body must carry. The endpoint depends on both the
// iac-coolify build_pack and the mode the request fields imply:
//
//   - dockerimage           → /applications/dockerimage (build pack implied by the path)
//   - dockerfile (inline)   → /applications/dockerfile  (a Dockerfile body, no git)
//   - dockerfile (git)      → /applications/public, build_pack "dockerfile"
//   - nixpacks/static/railpack → /applications/public, build_pack passed through
//   - docker-compose        → /applications/public, build_pack "dockercompose" (upstream spelling)
//
// An unknown build_pack is an error, never a guess.
func applicationCreateEndpoint(req CreateApplicationRequest) (endpoint, apiBuildPack string, err error) {
	switch req.BuildPack {
	case "dockerimage":
		return "/applications/dockerimage", "", nil
	case "dockerfile":
		if req.Dockerfile != "" {
			return "/applications/dockerfile", "dockerfile", nil
		}
		return "/applications/public", "dockerfile", nil
	case "nixpacks", "static", "railpack":
		return "/applications/public", req.BuildPack, nil
	case "docker-compose":
		return "/applications/public", "dockercompose", nil
	default:
		return "", "", fmt.Errorf(
			"coolify: cannot map build_pack %q to a Coolify v4 application endpoint (want dockerfile|dockerimage|nixpacks|docker-compose|static|railpack)",
			req.BuildPack)
	}
}

// validateCreatable reports whether req carries the source fields the chosen build_pack
// needs, returning an actionable error rather than letting a malformed request reach the
// API. dockerimage needs an image; dockerfile needs either inline content or a git source;
// every other (git-based) build_pack needs a git repository and branch.
func validateCreatable(req CreateApplicationRequest) error {
	switch req.BuildPack {
	case "dockerimage":
		if req.DockerRegistryImageName == "" {
			return fmt.Errorf("coolify: create application %q: docker_registry_image_name is required for build_pack dockerimage", req.Name)
		}
		return nil
	case "dockerfile":
		if req.Dockerfile == "" && (req.GitRepository == "" || req.GitBranch == "") {
			return notCreatableErr(req.BuildPack, "an inline Dockerfile, or a git repository and branch")
		}
		return nil
	case "nixpacks", "docker-compose", "static", "railpack":
		if req.GitRepository == "" || req.GitBranch == "" {
			return notCreatableErr(req.BuildPack, "a git repository and branch")
		}
		return nil
	default:
		_, _, err := applicationCreateEndpoint(req)
		return err
	}
}

func notCreatableErr(buildPack, needs string) error {
	return fmt.Errorf("coolify: create application: build_pack %q needs %s", buildPack, needs)
}

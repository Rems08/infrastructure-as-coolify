package coolify

import "fmt"

// applicationCreateTarget is the POST endpoint and body build_pack value for one
// iac-coolify build_pack.
type applicationCreateTarget struct {
	endpoint     string // path relative to /api/v1
	apiBuildPack string // value for the body's build_pack field; empty when the endpoint implies it
}

// applicationCreateTargets maps each iac-coolify build_pack to its Coolify v4 create
// endpoint. The /applications/dockerimage and /applications/dockerfile endpoints encode
// the build pack in the path; nixpacks and docker-compose share /applications/public and
// distinguish themselves through the body's build_pack field (dockercompose is the
// upstream spelling of docker-compose).
var applicationCreateTargets = map[string]applicationCreateTarget{
	"dockerimage":    {endpoint: "/applications/dockerimage"},
	"dockerfile":     {endpoint: "/applications/dockerfile", apiBuildPack: "dockerfile"},
	"nixpacks":       {endpoint: "/applications/public", apiBuildPack: "nixpacks"},
	"docker-compose": {endpoint: "/applications/public", apiBuildPack: "dockercompose"},
}

// ApplicationCreateEndpoint returns the Coolify v4 create endpoint (relative to /api/v1)
// for an iac-coolify build_pack, plus the build_pack value the request body must carry
// (empty when the endpoint implies it). An unknown build_pack is an error, never a guess.
func ApplicationCreateEndpoint(buildPack string) (endpoint, apiBuildPack string, err error) {
	t, ok := applicationCreateTargets[buildPack]
	if !ok {
		return "", "", fmt.Errorf(
			"coolify: cannot map build_pack %q to a Coolify v4 application endpoint (want dockerfile|dockerimage|nixpacks|docker-compose)",
			buildPack)
	}
	return t.endpoint, t.apiBuildPack, nil
}

// validateCreatable reports whether req carries the source fields the chosen build_pack
// needs. The iac-coolify Application schema currently models a prebuilt image only, so
// dockerimage is the one build_pack fully creatable today; the others need source fields
// (a Dockerfile body, or a git repository) the schema does not yet declare, and return an
// actionable error instead of a malformed request.
func validateCreatable(req CreateApplicationRequest) error {
	switch req.BuildPack {
	case "dockerimage":
		if req.DockerRegistryImageName == "" {
			return fmt.Errorf("coolify: create application %q: docker_registry_image_name is required for build_pack dockerimage", req.Name)
		}
		return nil
	case "dockerfile":
		if req.Dockerfile == "" {
			return notCreatableErr(req.BuildPack, "a Dockerfile body")
		}
		return nil
	case "nixpacks", "docker-compose":
		if req.GitRepository == "" || req.GitBranch == "" {
			return notCreatableErr(req.BuildPack, "a git repository and branch")
		}
		return nil
	default:
		_, _, err := ApplicationCreateEndpoint(req.BuildPack)
		return err
	}
}

func notCreatableErr(buildPack, needs string) error {
	return fmt.Errorf(
		"coolify: build_pack %q needs %s, which the iac-coolify Application schema does not yet model; only build_pack=dockerimage is currently creatable",
		buildPack, needs)
}

package container

import (
	"fmt"
	"testing"

	"github.com/docker/cli/e2e/internal/fixtures"
	"github.com/docker/cli/internal/test/environment"
	"gotest.tools/fs"
	"gotest.tools/icmd"
	"gotest.tools/skip"
)

func TestCreateWithContentTrust(t *testing.T) {
	skip.If(t, environment.RemoteDaemon())

	dir := fixtures.SetupConfigFile(t)
	defer dir.Remove()
	image := fixtures.CreateMaskedTrustedRemoteImage(t, registryPrefix, "trust-create", "latest")

	defer func() {
		icmd.RunCommand("docker", "image", "rm", image).Assert(t, icmd.Success)
	}()

	result := icmd.RunCmd(
		icmd.Command("docker", "create", image),
		fixtures.WithConfig(dir.Path()),
		fixtures.WithTrust,
		fixtures.WithNotary,
	)
	result.Assert(t, icmd.Expected{
		Err: fmt.Sprintf("Tagging %s@sha", image[:len(image)-7]),
	})
}

func TestTrustedCreateFromUnreachableTrustServer(t *testing.T) {
	dir := fixtures.SetupConfigFile(t)
	defer dir.Remove()
	image := fixtures.CreateMaskedTrustedRemoteImage(t, registryPrefix, "trust-create", "latest")

	result := icmd.RunCmd(
		icmd.Command("docker", "create", image),
		fixtures.WithConfig(dir.Path()),
		fixtures.WithTrust,
		fixtures.WithNotaryServer("https://invalidnotaryserver"),
	)
	result.Assert(t, icmd.Expected{
		ExitCode: 1,
		Err:      "error contacting notary server",
	})
}

func TestTrustedCreateFromBadTrustServer(t *testing.T) {
	dir := fixtures.SetupConfigFile(t)
	defer dir.Remove()
	image := fixtures.CreateTrustedRemoteImage(t, registryPrefix, "trust-create", "latest")

	// Try create
	icmd.RunCmd(
		icmd.Command("docker", "create", image),
		fixtures.WithConfig(dir.Path()),
		fixtures.WithTrust,
	).Assert(t, icmd.Success)
	icmd.RunCmd(icmd.Command("docker", "rmi", image)).Assert(t, icmd.Success)

	// Start evil notary server
	//	buildEvilNotary(t)
	startEvilNotary(t)

	defer func() {
		icmd.RunCmd(icmd.Command("docker", "stop", "evil-notary")).Assert(t, icmd.Success)
		icmd.RunCmd(icmd.Command("docker", "rm", "evil-notary")).Assert(t, icmd.Success)
	}()

	evilNotaryDir := setupEvilNotaryServer(t)
	defer evilNotaryDir.Remove()

	icmd.RunCmd(icmd.Command("docker", "create", image),
		fixtures.WithConfig(evilNotaryDir.Path()),
		fixtures.WithTrust,
		fixtures.WithNotaryServer("https://evil-notary:4443"),
	).Assert(t, icmd.Expected{
		ExitCode: 1,
		Err:      "error contacting notary server",
	})

}

func startEvilNotary(t *testing.T) {
	t.Helper()
	dir := fs.NewDir(t, "evil-notary",
		fs.FromDir("testdata"),
	)
	defer dir.Remove()

	icmd.RunCmd(icmd.Command("docker", "build", "-t", "evil-notary", "-f", "Dockerfile.notary-server"),
		fixtures.WithWorkingDir(dir)).Assert(t, icmd.Success)
	icmd.RunCmd(icmd.Command("docker", "run", "--name", "evil-notary", "-d", "-p", "4443:4443", fixtures.NotaryImage, "notary-server", "-config=/fixtures/notary-config.json"),
		fixtures.WithWorkingDir(dir)).Assert(t, icmd.Success)
}

func setupEvilNotaryServer(t *testing.T) fs.Dir {
	t.Helper()
	dir := fs.NewDir(t, "evil_notary_test", fs.WithMode(0700), fs.WithFile("config.json", `
	{
			"auths": {
					"registry:5000": {
							"auth": "ZWlhaXM6cGFzc3dvcmQK"
					},
					"https://evil-notary:4443": {
							"auth": "ZWlhaXM6cGFzc3dvcmQK"
					}
			},
			"experimental": "enabled"
	}
	`), fs.WithDir("trust", fs.WithDir("private")))
	return *dir
}

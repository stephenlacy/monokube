package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/fatih/color"
	"gopkg.in/yaml.v2"
)

var lernaRoot = "lerna.json"

// Package is the deployment spec for a monorepo package
type Package struct {
	Name        string
	Image       string
	Commit      string
	Version     string
	BuildDocker bool
	DockerArgs  string
	Path        string
	Manifests   []Manifest
	Env         map[string]string
	PackageConfig
}

// Manifest represents a k8s manifest config
type Manifest struct {
	File         string
	RunCondition string
	Kube         KubeManifest // unused
}

// KubeManifest represents the basic values needed from k8s
// unused
type KubeManifest struct {
	Name string
	Kind string
}

// LernaConfig is a basic representation of a lerna config file
type LernaConfig struct {
	Packages []string
}

// PackageConfig is a basic representation of a lerna config file
type PackageConfig struct {
	Version  string   `json:"version"`
	Clusters []string `json:"clusters"`
	Watch    bool     `json:"watch"`
	Kind     string   `json:"kind"`
}

var flagCommand = flag.String("command", "", "Specific command to run (build, deploy, post-deploy)")
var flagImageRoot = flag.String("image-root", "", "Docker image registry and root")
var flagDryRun = flag.Bool("dry-run", false, "Use flag --dry-run on kubectl")
var flagDockerArgs = flag.String("docker-args", "", "Docker build args '--build-arg'")
var flagSkipPackages = flag.String("skip-packages", "", "Skip provided packages '--skip-packages example-1 package-2'")
var flagOnlyPackages = flag.String("only-packages", "", "Only deploy provided packages '--only-packages example-1'")
var flagClusterName = flag.String("cluster-name", "", "Cluster name 'dev-cluster'")
var flagPath = flag.String("path", "", "Path for packages `packages`")
var flagDiff = flag.String("diff", "", "Diff between current commit and provided commit '--diff 0132547'")

var flagOutputDesc = "View output yaml"
var flagOutput = flag.String("output", "", flagOutputDesc)

func main() {
	var packages []Package

	// flagDiffCommit := flag.String("diff-commit", "", "Only build/deploy package if changed since provided git commit")
	flag.StringVar(flagOutput, "o", "", flagOutputDesc)

	flag.Parse()
	if *flagImageRoot == "" {
		log.Fatal("arg -image-root is required")
	}

	skippedPackages := strings.Fields(*flagSkipPackages)
	if len(skippedPackages) > 0 {
		color.Cyan("Skipping %v package(s)", len(skippedPackages))
	}

	onlyPackages := strings.Fields(*flagOnlyPackages)

	paths, err := getPaths()
	if err != nil {
		panic(err)
	}
	env := envToMap()
	rev, err := getCommit()
	if err != nil {
		rev = ""
		color.Red("error fetching git commit - continuing without")
	}

	// Assemble the list of packages in the project
OUTER:
	for _, pth := range paths {
		name := filepath.Base(pth)
		// Skip packages if flag is provided
		for _, v := range skippedPackages {
			if v == name {
				continue OUTER
			}
		}
		// Only deploy packages if flag is provided
		for _, v := range onlyPackages {
			if v != "" && v != name {
				continue OUTER
			}
		}
		if *flagDiff != "" {
			if !hasDiff(*flagDiff, pth) {
				color.Cyan("Package %v has not changed since commit %v", name, *flagDiff)
				continue OUTER
			}
		}
		pkg := Package{
			Name:        name,
			Commit:      rev,
			BuildDocker: checkFile(pth + "/Dockerfile"),
			Path:        pth,
			Env:         env,
		}
		dockerTpl, err := parseTemplate(*flagDockerArgs, pkg)
		if err != nil {
			color.Red("Arg '--docker-args' has an invalid template")
		}
		pkg.DockerArgs = dockerTpl

		kglob := parseGlobs([]string{pth + "/kube/*.yaml"})

		pkgCfg, err := getPackageConfig(pth)

		if err == nil {
			pkg.Version = pkgCfg.Version
			pkg.Clusters = pkgCfg.Clusters
			pkg.Kind = pkgCfg.Kind
		}
		pkg.Image = getImage(flagImageRoot, pkg)

		// Parse k8s templates
		pkg.Manifests = parseManifests(kglob, pkg)
		packages = append(packages, pkg)

	}
	if *flagDryRun {
		color.Cyan("dry-run is set")
	}

	if *flagCommand != "" && *flagCommand != "build" && *flagCommand != "deploy" && *flagCommand != "post-deploy" {
		color.Red("error: invalid command '%s'", *flagCommand)
	}

	// Build the docker images as needed
	if *flagCommand == "" || *flagCommand == "build" {
		color.Cyan("building %d package(s) \n", len(packages))
		for _, pkg := range packages {
			dockerPath := pkg.Path + "/Dockerfile"
			if !pkg.BuildDocker {
				continue
			}
			cmd := fmt.Sprintf("docker build %s -t %s -f %s %s", pkg.DockerArgs, pkg.Image, dockerPath, pkg.Path)
			fmt.Println(cmd)
			err := runBackground(pkg, "bash", "-c", cmd)
			if err != nil {
				color.Red("error building image %s %e \n", pkg.Image, err)
				break
			}
			color.Cyan("built image: %s\n", pkg.Image)

			if *flagDryRun {
				color.Yellow("not pushing docker images as dry-run is set")
				continue
			}
			err = runBackground(pkg, "docker", "push", pkg.Image)
			if err != nil {
				color.Red("error pushing image %s %e \n", pkg.Image, err)
				break
			}
		}
	}

	// All images are built and pushed - now start the kube rollout
	if *flagCommand == "" || *flagCommand == "deploy" {
		color.Cyan("running deployments")
		applyManifests(packages, "normal")
	}
	// Run the post-deploy tasks
	if *flagCommand == "" || *flagCommand == "post-deploy" {
		color.Cyan("running post-deployment tasks")
		applyManifests(packages, "post-deploy")
	}
	color.Green("all done")
}

func applyManifests(packages []Package, runCondition string) {
	for _, pkg := range packages {
		dryRun := ""
		output := ""
		if *flagDryRun {
			dryRun = " --dry-run"
		}
		if *flagOutput != "" {
			output = fmt.Sprintf(" --output %s", *flagOutput)
		}

	OUTER:
		for _, manifest := range pkg.Manifests {
			// Run at end
			if manifest.RunCondition != runCondition {
				continue
			}
			// Check if package has a Cluster specified
			// Note: if a package does not have the `cluster` specified it will be deployed always
			if len(pkg.Clusters) > 0 {
				if *flagClusterName == "" {
					color.Yellow("package %s has clusters provided but --cluster-name not provided", pkg.Name)
				} else {
					found := false
					for _, v := range pkg.Clusters {
						if *flagClusterName == v {
							found = true
						}
					}
					if !found {
						color.Yellow("cluster name mismatch - cluster %s not found in %s config", *flagClusterName, pkg.Name)
						continue OUTER
					}
				}

			}
			err := runBackground(pkg, "bash", "-c", fmt.Sprintf("echo '%s' | kubectl apply %s%s -f -", manifest.File, output, dryRun))
			if err != nil {
				color.New(color.FgRed).Add(color.Bold).Printf("error deploying %s %e \n", pkg.Name, err)
				break
			}

			if !*flagDryRun {
				runBackground(pkg, "kubectl", "rollout", "status", pkg.Kind+"/"+pkg.Name)
			}
		}
	}
}

func parseManifests(paths []string, pkg Package) []Manifest {
	var manifests []Manifest
	for _, pth := range paths {
		f, err := ioutil.ReadFile(pth)
		runCondition := "normal"
		if filepath.Base(pth) == "post-deploy.yaml" {
			runCondition = "post-deploy"
		}
		if err != nil {
			color.Red("error reading %s: %e \n", pth, err)
			return []Manifest{}
		}
		str, err := parseTemplate(string(f), pkg)
		if err != nil {
			color.Red("error parsing %s: %e \n", pth, err)
			continue
		}
		m := KubeManifest{}
		_ = yaml.Unmarshal([]byte(str), &m)
		mani := Manifest{File: str, RunCondition: runCondition, Kube: m}
		manifests = append(manifests, mani)
	}
	return manifests
}

func parseTemplate(input string, pkg Package) (string, error) {
	t, err := template.New("pkg").Parse(input)
	if err != nil {
		return "", err
	}
	var tpl bytes.Buffer
	err = t.Execute(&tpl, pkg)
	return tpl.String(), err
}

func getPaths() ([]string, error) {
	lernaCfg := LernaConfig{}

	if *flagPath != "" {
		return parseGlobs([]string{fmt.Sprintf("%s/*", *flagPath)}), nil
	}
	if checkFile(lernaRoot) {
		f, err := ioutil.ReadFile(lernaRoot)
		if err != nil {
			return []string{}, err
		}
		json.Unmarshal(f, &lernaCfg)
		return parseGlobs(lernaCfg.Packages), nil
	}
	return parseGlobs([]string{"packages/*"}), nil
}

func getPackageConfig(pth string) (PackageConfig, error) {
	cfg0 := PackageConfig{}

	jsn0 := pth + "/package.json"
	jsn1 := pth + "/kube/deploy.json"
	if checkFile(jsn0) {
		f, err := ioutil.ReadFile(jsn0)
		if err != nil {
			return PackageConfig{}, err
		}
		json.Unmarshal(f, &cfg0)
	}
	if checkFile(jsn1) {
		f, err := ioutil.ReadFile(jsn1)
		if err != nil {
			return PackageConfig{}, err
		}
		json.Unmarshal(f, &cfg0)
	}
	if cfg0.Kind == "" {
		cfg0.Kind = "deployment"
	}
	return cfg0, nil
}

func parseGlobs(paths []string) []string {
	joined := []string{}
	for _, path := range paths {
		pth, err := filepath.Glob(path)
		if err != nil {
			continue
		}
		joined = append(joined, pth...)
	}
	return joined
}

func checkFile(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func getImage(imageRoot *string, pkg Package) string {
	commit := ""
	if pkg.Commit != "" {
		commit = "-" + pkg.Commit
	}
	return strings.TrimSuffix(fmt.Sprintf("%s/%s:%s%s", *imageRoot, pkg.Name, pkg.Version, commit), "\n")
}

func runBackground(pkg Package, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return err
}

func runOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}

func getCommit() (string, error) {
	return runOutput("git", "rev-parse", "--short", "HEAD")
}

func hasDiff(commit, path string) bool {
	diff, err := runOutput("bash", "-c", fmt.Sprintf("git diff --name-status %v | grep %v", commit, path))
	if err != nil {
		return false
	}
	return diff != ""
}

func envToMap() map[string]string {
	env := os.Environ()
	mapped := map[string]string{}
	for _, v := range env {
		s := strings.Split(v, "=")
		mapped[s[0]] = s[1]
	}
	return mapped
}

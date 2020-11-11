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
}

// LernaConfig is a basic representation of a lerna config file
type LernaConfig struct {
	Packages []string
}

// PackageConfig is a basic representation of a lerna config file
type PackageConfig struct {
	Version  string   `json:"version"`
	Clusters []string `json:"clusters"`
}

func main() {
	var packages []Package

	flagImageRoot := flag.String("image-root", "", "Docker image registry and root")
	flagDryRun := flag.Bool("dry-run", false, "Use flag --dry-run on kubectl")
	flagDockerArgs := flag.String("docker-args", "", "Docker build args '--build-arg'")
	// flagDiffCommit := flag.String("diff-commit", "", "Only build/deploy package if changed since provided git commit")
	flagOutputDesc := "View output yaml"
	flagOutput := flag.String("output", "", flagOutputDesc)
	flag.StringVar(flagOutput, "o", "", flagOutputDesc)

	flag.Parse()
	if *flagImageRoot == "" {
		log.Fatal("arg -image-root is required")
	}

	paths, err := getLernaConfig()
	if err != nil {
		panic(err)
	}
	rev, err := getCommit()
	env := envToMap()
	if err != nil {
		color.Red("error fetching git commit - continuing without")
	}

	// assemble the list of packages in the project
	for _, pth := range paths {
		name := filepath.Base(pth)
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
		}
		pkg.Image = getImage(flagImageRoot, pkg)

		// parse k8s templates
		pkg.Manifests = parseManifests(kglob, pkg)
		packages = append(packages, pkg)

	}
	color.Cyan("discovered %d package(s) \n", len(packages))
	if *flagDryRun {
		color.Cyan("dry-run is set")
	}

	// build the docker images as needed
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
		color.Green("built image: %s\n", pkg.Image)

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

	// all images are built and pushed - now start the kube rollout

	applyManifests(packages, "normal", flagDryRun, flagOutput)
	applyManifests(packages, "post-deploy", flagDryRun, flagOutput)
}

func applyManifests(packages []Package, runCondition string, flagDryRun *bool, flagOutput *string) {
	for _, pkg := range packages {
		dryRun := ""
		output := ""
		if *flagDryRun {
			dryRun = " --dry-run"
		}
		if *flagOutput != "" {
			output = fmt.Sprintf(" --output %s", *flagOutput)
		}

		for _, manifest := range pkg.Manifests {
			// run at end
			if manifest.RunCondition != runCondition {
				continue
			}
			err := runBackground(pkg, "bash", "-c", fmt.Sprintf("echo '%s' | kubectl apply %s%s -f -", manifest.File, output, dryRun))
			if err != nil {
				color.New(color.FgRed).Add(color.Bold).Printf("error deploying %s %e \n", pkg.Name, err)
				break
			}

			if !*flagDryRun {
				runBackground(pkg, "kubectl", "rollout", "status", "deployment/"+pkg.Name)
			}
		}
	}
}

func parseManifests(paths []string, pkg Package) []Manifest {
	var manifests []Manifest
	for _, pth := range paths {
		f, err := ioutil.ReadFile(pth)
		runCondition := "normal"
		if pth == "post-deploy.yaml" {
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
		manifests = append(manifests, Manifest{File: str, RunCondition: runCondition})
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

func getLernaConfig() ([]string, error) {
	lernaCfg := LernaConfig{}

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
	return fmt.Sprintf("%s/%s:%s%s", *imageRoot, pkg.Name, pkg.Version, commit)
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

func envToMap() map[string]string {
	env := os.Environ()
	mapped := map[string]string{}
	for _, v := range env {
		s := strings.Split(v, "=")
		mapped[s[0]] = s[1]
	}
	return mapped
}

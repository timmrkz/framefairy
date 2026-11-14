package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"framefairy/updates"
)

// A plan is what a workflow needs to know to go on: whether to build or
// publish, and what. It is written as the step's outputs, one name and
// one value a line, and said in a line of its own when there is nothing
// to do.
type plan struct {
	outputs [][2]string
	notice  string
}

func (p *plan) set(name, value string) {
	// One line a value: a title can hold anything.
	value = strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
	p.outputs = append(p.outputs, [2]string{name, value})
}

// write appends the outputs to the file GitHub reads them from, and says
// the notice where the run shows it.
func (p plan) write(out io.Writer, outputs string) error {
	if p.notice != "" {
		fmt.Fprintf(out, "::notice::%s\n", strings.Join(strings.Fields(p.notice), " "))
	}
	if outputs == "" {
		return errors.New("GITHUB_OUTPUT is not set, so there is nowhere to say the plan")
	}
	f, err := os.OpenFile(outputs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	for _, kv := range p.outputs {
		if _, err := fmt.Fprintf(f, "%s=%s\n", kv[0], kv[1]); err != nil {
			f.Close()
			return err
		}
		fmt.Fprintf(out, "%s: %s\n", kv[0], kv[1])
	}
	return f.Close()
}

// hasKey says whether the repository holds the public half of the update
// key. Until it does, nothing is built or published.
func hasKey() bool {
	text, err := os.ReadFile(publicKeyFile)
	return err == nil && strings.TrimSpace(string(text)) != ""
}

var versionLine = regexp.MustCompile(`(?m)^const Version = "([0-9]+\.[0-9]+\.[0-9]+)"`)

// engineVersion is the engine's version as engine/doc.go says it, read
// and not run.
func engineVersion(doc []byte) (string, error) {
	m := versionLine.FindSubmatch(doc)
	if m == nil {
		return "", errors.New("engine/doc.go says no version")
	}
	return string(m[1]), nil
}

// versionOf is a build's version: the engine's, the channel's tag and the
// commit, shortened the way git and GitHub shorten it, so the version
// says what is in the build. builds.yml and publish.yml both name a build
// by it, and what the app says it is and what the list says it is are one.
func versionOf(engine, channel, commit string) string {
	tag := strings.ReplaceAll(channel, "-", "")
	return engine + "-" + tag + "." + commit[:min(len(commit), 7)]
}

// pullOf is the open pull request of a branch, if it has one.
func pullOf(gh gitHub, branch string) (pull, bool, error) {
	all, err := gh.pulls()
	if err != nil {
		return pull{}, false, err
	}
	for _, p := range all {
		if p.branch == branch {
			return p, true, nil
		}
	}
	return pull{}, false, nil
}

// builtCommit is the commit a channel's build was made from, as its entry
// in the release says, or empty when it has none.
func builtCommit(gh gitHub, channel string) (string, error) {
	all, err := gh.assets()
	if err != nil {
		return "", err
	}
	for _, a := range all {
		if a.name != "channel-"+channel+".json" {
			continue
		}
		data, err := gh.download(a)
		if err != nil {
			return "", err
		}
		var b updates.Build
		if err := json.Unmarshal(data, &b); err != nil {
			return "", nil
		}
		return b.Commit, nil
	}
	return "", nil
}

// buildPlan says what a push is a build of, for builds.yml, and whether
// it needs one. A push to main is built. A push to a branch of this
// repository with an open pull request is built for that pull request,
// unless its build is of the same files, or it only changes the docs
// against it. Anything else, a closed pull request among them, builds
// nothing. engineDoc is engine/doc.go at the commit pushed.
func buildPlan(gh gitHub, t trees, event, branch, commit string, engineDoc []byte) (plan, error) {
	var p plan
	no := func(why string) (plan, error) {
		p.notice = why
		p.set("build", "false")
		return p, nil
	}
	if event != "push" {
		return no("Only a push builds. A pull request that closes only has the channel list written again.")
	}
	if !hasKey() {
		return no("No update key yet, so nothing is built to update to. See docs/UPDATES.md, The keys.")
	}
	channel, number := "main", ""
	if branch != "main" {
		pr, ok, err := pullOf(gh, branch)
		if err != nil {
			return p, err
		}
		if !ok {
			return no(branch + " has no open pull request, so it has no channel to build for.")
		}
		channel, number = fmt.Sprintf("pr-%d", pr.number), fmt.Sprint(pr.number)
		// main is always built. A pull request's commit that is built
		// already, or only changes the docs against its build, is not, and
		// Tim gets no restart for nothing.
		// A build that cannot be read is no build, so a doubt builds, the
		// way every doubt in needsBuild does.
		from, err := builtCommit(gh, channel)
		if err != nil {
			p.notice = "The build there is could not be read, so this commit is built: " + err.Error()
			from = ""
		}
		if answer, why := needsBuild(t, from, commit); answer != needed {
			return no(why)
		}
	}
	engine, err := engineVersion(engineDoc)
	if err != nil {
		return p, err
	}
	p.set("build", "true")
	p.set("channel", channel)
	p.set("number", number)
	p.set("sha", commit)
	p.set("version", versionOf(engine, channel, commit))
	p.set("commit", commit[:min(len(commit), 12)])
	return p, nil
}

// publishPlan says what a finished build is, for publish.yml, from
// GitHub's record of the run and nothing the build wrote: which channel,
// which version, which commit. A run that left no zip is a commit that
// needed no build, and there is nothing to publish. A branch whose pull
// request closed has no channel any more. The commit has to be one its
// channel's history holds, see held.
func publishPlan(gh gitHub, run, branch, commit string) (plan, error) {
	var p plan
	no := func(why string) (plan, error) {
		p.notice = why
		p.set("run", "false")
		return p, nil
	}
	zips, err := gh.builds(run)
	if err != nil {
		return p, err
	}
	if zips == 0 {
		return no("The run left no build, so there is nothing to publish.")
	}
	if !hasKey() {
		return no("No update key yet, so nothing is published. See docs/UPDATES.md, The keys.")
	}
	channel, name, head := "main", "main", ""
	if branch != "main" {
		pr, ok, err := pullOf(gh, branch)
		if err != nil {
			return p, err
		}
		if !ok {
			return no(branch + " has no open pull request any more, so its build has no channel.")
		}
		channel, head = fmt.Sprintf("pr-%d", pr.number), pr.head
		title, _, _ := strings.Cut(pr.title, "\n")
		name = fmt.Sprintf("#%d %s", pr.number, strings.TrimSpace(title))
	}
	if err := held(gh, channel, commit, head); err != nil {
		return p, err
	}
	// A run of an older commit, run again, or a tag named main at an old
	// commit of main, would put an older build in place of the newer one.
	if built, err := builtCommit(gh, channel); err == nil && built != "" && !strings.HasPrefix(commit, built) {
		if status, err := gh.compare(commit, built); err == nil && status == "ahead" {
			return no(fmt.Sprintf("%s has a newer build already, of %s, so this older one is not published.", channel, built))
		}
	}
	doc, err := gh.fileAt("engine/doc.go", commit)
	if err != nil {
		return p, fmt.Errorf("the engine's version at %s cannot be read: %w", commit, err)
	}
	engine, err := engineVersion(doc)
	if err != nil {
		return p, err
	}
	version := versionOf(engine, channel, commit)
	p.set("run", "true")
	p.set("channel", channel)
	p.set("name", name)
	p.set("version", version)
	p.set("commit", commit[:min(len(commit), 12)])
	p.set("zip", "app-"+channel+"-"+version+".zip")
	return p, nil
}

// held says whether a build of commit belongs to its channel: main's
// history holds it, or the history of the pull request's newest commit,
// head. GitHub reports a tag pushed with the name main, or with a pull
// request's branch, as a push to that branch, at any commit, so the
// record of the run alone is not enough, #838.
func held(gh gitHub, channel, commit, head string) error {
	if channel == "main" {
		var err error
		if head, err = gh.mainCommit(); err != nil {
			return fmt.Errorf("main's commit cannot be read, so %s is not known to be main's: %w", commit, err)
		}
	}
	if head == "" {
		return fmt.Errorf("say the newest commit of %s, so %s can be compared with it", channel, commit)
	}
	status, err := gh.compare(commit, head)
	if err != nil {
		return fmt.Errorf("%s cannot be compared with %s's newest commit, so it is not known to be %s's: %w", commit, channel, channel, err)
	}
	if status == "identical" || status == "ahead" {
		return nil
	}
	if channel == "main" {
		return fmt.Errorf("%s is not in main's history, so it is not signed as main's build. A tag named main may have been pushed: look at the tags of the repository", commit)
	}
	// A branch rewritten after the push, as Dependabot does when it
	// rebases, is the usual reason. Its newer push builds by itself.
	return fmt.Errorf("%s is not in the history of %s's newest commit, %s, so it is not published for it", commit, channel, head)
}

// decodeBase64Lines reads base64 the way GitHub sends a file, broken
// into lines.
func decodeBase64Lines(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.NewReplacer("\n", "", "\r", "").Replace(s))
}

// planCommand runs buildPlan or publishPlan from the flags a workflow
// step gives, and writes the plan where GitHub reads it.
func planCommand(name string, args []string, gh gitHub) error {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	event := fs.String("event", "", "the event that started the run, push or pull_request")
	branch := fs.String("branch", "", "the branch pushed")
	commit := fs.String("commit", "", "the commit built")
	run := fs.String("run", "", "the run of builds.yml that made the build")
	_ = fs.Parse(args)
	if *commit == "" {
		return errors.New("say which commit with -commit")
	}
	var p plan
	var err error
	if name == "build-plan" {
		doc, rerr := os.ReadFile("engine/doc.go")
		if rerr != nil {
			return rerr
		}
		p, err = buildPlan(gh, gitTrees{gh}, *event, *branch, *commit, doc)
	} else {
		p, err = publishPlan(gh, *run, *branch, *commit)
	}
	if err != nil {
		return err
	}
	return p.write(os.Stdout, os.Getenv("GITHUB_OUTPUT"))
}

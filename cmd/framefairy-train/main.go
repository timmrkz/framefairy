// Command framefairy-train works with the training records the app and the
// framefairy command write next to each episode. It is a tool for improving the
// clip selection model and is not shipped with the app.
//
//	framefairy-train mark episode.mp4 --keep 01,04 --reject 02,07 --reason 02=no-payoff --published 04
//	framefairy-train export --root ~/Podcast --out dataset --eval-share 0.2
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"framefairy/engine"
	"framefairy/train"
)

const usage = `usage:
  framefairy-train mark EPISODE [--plan FILE] [--keep IDS] [--reject IDS] [--reason ID=REASON,...] [--published IDS] [--unpublished IDS]
  framefairy-train export --out DIR [--root DIR] [--eval-share 0.2]

The records of every episode are in one folder, ~/.framefairy/training unless
FRAMEFAIRY_TRAINING or the app's setting says otherwise. --root reads somewhere
else instead, which also finds the records that older versions kept beside
each episode.

Reasons: ` + "no-payoff, weak-opening, needs-context, too-long, too-short, bad-cut, boring, other"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "mark":
		err = mark(os.Args[2:])
	case "export":
		err = export(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Println(usage)
		return
	default:
		err = fmt.Errorf("unknown command %s\n%s", os.Args[1], usage)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "framefairy-train:", err)
		os.Exit(1)
	}
}

func ids(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// splitArgs lets the episode come before or after the flags.
func splitArgs(args []string) (string, []string) {
	var positional string
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			if !strings.Contains(a, "=") && i+1 < len(args) {
				rest = append(rest, args[i+1])
				i++
			}
			continue
		}
		positional = a
	}
	return positional, rest
}

func mark(args []string) error {
	episode, rest := splitArgs(args)
	fs := flag.NewFlagSet("mark", flag.ContinueOnError)
	plan := fs.String("plan", "", "plan file, defaults to the episode's only plan")
	keep := fs.String("keep", "", "clip ids to keep")
	reject := fs.String("reject", "", "clip ids to reject")
	reason := fs.String("reason", "", "reasons for rejections, as ID=REASON pairs")
	published := fs.String("published", "", "clip ids that were published")
	unpublished := fs.String("unpublished", "", "clip ids that were taken down")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if episode == "" {
		return fmt.Errorf("name the episode\n%s", usage)
	}
	planPath := *plan
	if planPath == "" {
		plans := engine.PlanSummaries(filepath.Join(engine.WorkDir(episode), "logs"))
		if len(plans) != 1 {
			return fmt.Errorf("%d plans found for %s, choose one with --plan", len(plans), episode)
		}
		planPath = plans[0].Path
	}
	reasons := map[string][]string{}
	for _, pair := range ids(*reason) {
		id, why, ok := strings.Cut(pair, "=")
		if !ok {
			return fmt.Errorf("--reason wants ID=REASON, got %s", pair)
		}
		reasons[id] = append(reasons[id], why)
	}
	for _, id := range ids(*keep) {
		if err := engine.SetRejected(planPath, id, false); err != nil {
			return err
		}
	}
	for _, id := range ids(*reject) {
		if err := engine.SetRejected(planPath, id, true, reasons[id]...); err != nil {
			return err
		}
	}
	for _, id := range ids(*published) {
		if err := engine.RecordDecision(planPath, id, engine.DecisionPublished, nil); err != nil {
			return err
		}
	}
	for _, id := range ids(*unpublished) {
		if err := engine.RecordDecision(planPath, id, engine.DecisionUnpublished, nil); err != nil {
			return err
		}
	}
	fmt.Println("recorded in", engine.TrainingDir())
	return nil
}

func export(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	root := fs.String("root", "", "folder to read the records from, default the one training folder")
	out := fs.String("out", "", "folder to write the dataset to")
	share := fs.Float64("eval-share", 0.2, "share of episodes held out for evaluation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("--out is needed\n%s", usage)
	}
	from := *root
	if from == "" {
		from = engine.TrainingDir()
	}
	episodes, err := train.FindEpisodes(from)
	if err != nil {
		return err
	}
	if len(episodes) == 0 {
		return fmt.Errorf("no training records in %s", from)
	}
	m, err := train.Export(episodes, *out, *share)
	if err != nil {
		return err
	}
	fmt.Printf("%d episode folders, %d examples from %d plan records and %d decision records\n",
		len(episodes), m.Counts["examples"], m.Counts["plan_records"], m.Counts["decision_records"])
	fmt.Printf("%d full hits, %d changed, %d published, %d rendered, %d kept, %d rejected\n",
		m.Counts["full_hits"], m.Counts["changed"], m.Counts["published"], m.Counts["rendered"],
		m.Counts["kept"], m.Counts["rejected"])
	fmt.Printf("train: %d sft, %d dpo   eval: %d sft, %d dpo\n",
		m.Counts["train_sft"], m.Counts["train_dpo"], m.Counts["eval_sft"], m.Counts["eval_dpo"])
	fmt.Println("written to", *out)
	return nil
}

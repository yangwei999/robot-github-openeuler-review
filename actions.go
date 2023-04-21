package main

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	gc "github.com/opensourceways/robot-github-lib/client"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	sdk "github.com/google/go-github/v36/github"
)

const (
	retestCommand       = "/retest"
	removeClaCommand    = "/cla cancel"
	rebaseCommand       = "/rebase"
	removeRebase        = "/rebase cancel"
	removeSquash        = "/squash cancel"
	baseMergeMethod     = "merge"
	squashCommand       = "/squash"
	removeLabel         = "openeuler-cla/yes"
	ackLabel            = "Acked"
	msgNotSetReviewer   = "**@%s** Thank you for submitting a PullRequest. It is detected that you have not set a reviewer, please set a one."
	sourceBranchChanged = "synchronize"
	open                = "open"
	updateLabel         = "labeled"
)

var (
	regAck     = regexp.MustCompile(`(?mi)^/ack\s*$`)
	ackCommand = regexp.MustCompile(`(?mi)^/ack\s*$`)
)

func (bot *robot) removeInvalidCLA(e *sdk.IssueCommentEvent, cfg *botConfig, log *logrus.Entry) error {
	if !e.GetIssue().IsPullRequest() ||
		e.GetIssue().GetState() != open ||
		!gc.IsCommentCreated(e) ||
		e.GetComment().GetBody() != removeClaCommand {
		return nil
	}

	org, repo := gc.GetOrgRepo(e.GetRepo())
	number := e.GetIssue().GetNumber()
	commenter := e.GetComment().GetUser().GetLogin()

	hasPermission, err := bot.hasPermission(org, repo, commenter, false, e, cfg, log)
	if err != nil {
		return err
	}

	if !hasPermission {
		return nil
	}

	return bot.cli.RemovePRLabel(gc.PRInfo{Org: org, Repo: repo, Number: number}, removeLabel)
}

func (bot *robot) handleRebase(e *sdk.IssueCommentEvent, cfg *botConfig, log *logrus.Entry) error {
	if !e.GetIssue().IsPullRequest() ||
		e.GetIssue().GetState() != open ||
		!gc.IsCommentCreated(e) ||
		e.GetComment().GetBody() != rebaseCommand {
		return nil
	}

	org, repo := gc.GetOrgRepo(e.GetRepo())
	number := e.GetIssue().GetNumber()
	commenter := e.GetComment().GetUser().GetLogin()

	hasPermission, err := bot.hasPermission(org, repo, commenter, false, e, cfg, log)
	if err != nil {
		return err
	}

	if !hasPermission {
		return nil
	}

	var prLabels map[string]string
	for _, l := range e.GetIssue().Labels {
		prLabels[l.GetName()] = l.GetName()
	}
	if _, ok := prLabels["merge/squash"]; ok {
		return bot.cli.CreatePRComment(gc.PRInfo{Org: org, Repo: repo, Number: number},
			"Please use **/squash cancel** to remove **merge/squash** label, and try **/rebase** again")
	}

	return bot.cli.AddPRLabel(gc.PRInfo{Org: org, Repo: repo, Number: number}, "merge/rebase")
}

func (bot *robot) handleFlattened(e *sdk.IssueCommentEvent, cfg *botConfig, log *logrus.Entry) error {
	if !e.GetIssue().IsPullRequest() ||
		e.GetIssue().GetState() != open ||
		!gc.IsCommentCreated(e) ||
		e.GetComment().GetBody() != squashCommand {
		return nil
	}

	org, repo := gc.GetOrgRepo(e.GetRepo())
	number := e.GetIssue().GetNumber()
	commenter := e.GetComment().GetUser().GetLogin()

	hasPermission, err := bot.hasPermission(org, repo, commenter, false, e, cfg, log)
	if err != nil {
		return err
	}

	if !hasPermission {
		return nil
	}

	var prLabels map[string]string
	for _, l := range e.GetIssue().Labels {
		prLabels[l.GetName()] = l.GetName()
	}
	if _, ok := prLabels["merge/rebase"]; ok {
		return bot.cli.CreatePRComment(gc.PRInfo{Org: org, Repo: repo, Number: number},
			"Please use **/rebase cancel** to remove **merge/rebase** label, and try **/squash** again")
	}

	return bot.cli.AddPRLabel(gc.PRInfo{Org: org, Repo: repo, Number: number}, "merge/squash")
}

func (bot *robot) doRetest(e *sdk.PullRequestEvent, p gc.PRInfo) error {
	if e.GetAction() != sourceBranchChanged || e.GetPullRequest().GetState() != open {
		return nil
	}

	return bot.cli.CreatePRComment(p, retestCommand)
}

func (bot *robot) checkReviewer(e *sdk.PullRequestEvent, p gc.PRInfo, cfg *botConfig) error {
	if cfg.UnableCheckingReviewerForPR || e.GetPullRequest().GetState() != open {
		return nil
	}

	if e.GetPullRequest() != nil && len(e.GetPullRequest().Assignees) > 0 {
		return nil
	}

	return bot.cli.CreatePRComment(p, fmt.Sprintf(msgNotSetReviewer, e.GetPullRequest().GetUser().GetLogin()))
}

func (bot *robot) clearLabel(e *sdk.PullRequestEvent, p gc.PRInfo) error {
	if e.GetAction() != sourceBranchChanged || e.GetPullRequest().GetState() != open {
		return nil
	}

	labels := sets.NewString()
	for _, l := range e.GetPullRequest().Labels {
		labels.Insert(*l.Name)
	}
	v := getLGTMLabelsOnPR(labels)

	if labels.Has(approvedLabel) {
		v = append(v, approvedLabel)
	}

	if len(v) > 0 {
		for _, vv := range v {
			if err := bot.cli.RemovePRLabel(p, vv); err != nil {
				return err
			}
		}

		return bot.cli.CreatePRComment(p, fmt.Sprintf(commentClearLabel, strings.Join(v, ", ")))
	}

	return nil
}

func (bot *robot) genMergeMethod(e *sdk.PullRequest, org, repo string, log *logrus.Entry) string {
	mergeMethod := "merge"

	prLabels := e.Labels
	sigLabel := ""

	for _, p := range prLabels {
		if strings.HasPrefix(p.GetName(), "merge/") {
			if strings.Split(p.GetName(), "/")[1] == "squash" {
				return "squash"
			}

			return strings.Split(p.GetName(), "/")[1]
		}

		if strings.HasPrefix(p.GetName(), "sig/") {
			sigLabel = p.GetName()
		}
	}

	if sigLabel == "" {
		return mergeMethod
	}

	sig := strings.Split(sigLabel, "/")[1]
	filePath := fmt.Sprintf("sig/%s/%s/%s/%s", sig, org, strings.ToLower(repo[0:1]), fmt.Sprintf("%s.yaml", repo))

	c, err := bot.cli.GetPathContent("openeuler", "community", filePath, "master")
	if err != nil {
		log.Infof("get repo %s failed, because of %v", fmt.Sprintf("%s-%s", org, repo), err)

		return mergeMethod
	}

	mergeMethod = bot.decodeRepoYaml(c, log)

	return mergeMethod
}

func (bot *robot) removeRebase(e *sdk.IssueCommentEvent, cfg *botConfig, log *logrus.Entry) error {
	if !e.GetIssue().IsPullRequest() ||
		e.GetIssue().GetState() != open ||
		!gc.IsCommentCreated(e) ||
		e.GetComment().GetBody() != removeRebase {
		return nil
	}

	org, repo := gc.GetOrgRepo(e.GetRepo())
	number := e.GetIssue().GetNumber()
	commenter := e.GetComment().GetUser().GetLogin()

	hasPermission, err := bot.hasPermission(org, repo, commenter, false, e, cfg, log)
	if err != nil {
		return err
	}

	if !hasPermission {
		return nil
	}

	return bot.cli.RemovePRLabel(gc.PRInfo{Org: org, Repo: repo, Number: number}, "merge/rebase")
}

func (bot *robot) removeFlattened(e *sdk.IssueCommentEvent, cfg *botConfig, log *logrus.Entry) error {
	if !e.GetIssue().IsPullRequest() ||
		e.GetIssue().GetState() != open ||
		!gc.IsCommentCreated(e) ||
		e.GetComment().GetBody() != removeSquash {
		return nil
	}

	org, repo := gc.GetOrgRepo(e.GetRepo())
	number := e.GetIssue().GetNumber()
	commenter := e.GetComment().GetUser().GetLogin()

	hasPermission, err := bot.hasPermission(org, repo, commenter, false, e, cfg, log)
	if err != nil {
		return err
	}

	if !hasPermission {
		return nil
	}

	return bot.cli.RemovePRLabel(gc.PRInfo{Org: org, Repo: repo, Number: number}, "merge/squash")
}

func (bot *robot) handleACK(e *sdk.IssueCommentEvent, cfg *botConfig, log *logrus.Entry) error {
	if !e.GetIssue().IsPullRequest() ||
		e.GetIssue().GetState() != open ||
		!gc.IsCommentCreated(e) {
		return nil
	}

	if !ackCommand.MatchString(e.GetComment().GetBody()) {
		return nil
	}

	org, repo := gc.GetOrgRepo(e.GetRepo())
	if org != "openeuler" && repo != "kernel" {
		return nil
	}

	number := e.GetIssue().GetNumber()
	commenter := e.GetComment().GetUser().GetLogin()

	hasPermission, err := bot.hasPermission(org, repo, commenter, false, e, cfg, log)
	if err != nil {
		return err
	}

	if !hasPermission {
		return nil
	}

	return bot.cli.AddPRLabel(gc.PRInfo{Org: org, Repo: repo, Number: number}, ackLabel)
}

func (bot *robot) decodeRepoYaml(content *sdk.RepositoryContent, log *logrus.Entry) string {
	c, err := base64.StdEncoding.DecodeString(*content.Content)
	if err != nil {
		log.WithError(err).Error("decode file")

		return baseMergeMethod
	}

	var r Repository
	if err = yaml.Unmarshal(c, &r); err != nil {
		log.WithError(err).Error("code yaml file")

		return baseMergeMethod
	}

	if r.MergeMethod != "" {
		if r.MergeMethod == "rebase" || r.MergeMethod == "squash" {
			return r.MergeMethod
		}
	}

	return baseMergeMethod
}

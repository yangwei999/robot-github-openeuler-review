package main

import (
	"fmt"
	"regexp"
	"strings"

	gc "github.com/opensourceways/robot-github-lib/client"

	sdk "github.com/google/go-github/v36/github"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// the github platform limits the maximum length of label to 20.
	labelLenLimit = 20
	lgtmLabel     = "lgtm"

	commentAddLGTMBySelf            = "***lgtm*** can not be added in your self-own pull request. :astonished:"
	commentClearLabel               = `New code changes of pr are detected and remove these labels ***%s***. :flushed: `
	commentNoPermissionForLgtmLabel = `Thanks for your review, ***%s***, your opinion is very important to us.:wave:
The maintainers will consider your advice carefully.`
	commentNoPermissionForLabel = `
***@%s*** has no permission to %s ***%s*** label in this pull request. :astonished:
Please contact to the collaborators in this repository.`
	commentAddLabel = `***%s*** was added to this pull request by: ***%s***. :wave: 
**NOTE:** If this pull request is not merged while all conditions are met, comment "/check-pr" to try again. :smile: `
	commentRemovedLabel = `***%s*** was removed in this pull request by: ***%s***. :flushed: `
)

var (
	regAddLgtm    = regexp.MustCompile(`(?mi)^/lgtm\s*$`)
	regRemoveLgtm = regexp.MustCompile(`(?mi)^/lgtm cancel\s*$`)
)

func (bot *robot) handleLGTM(e *sdk.IssueCommentEvent, cfg *botConfig, log *logrus.Entry) error {
	if !e.GetIssue().IsPullRequest() || !(e.GetIssue().GetState() == open) || !gc.IsCommentCreated(e) {
		return nil
	}

	comment := e.GetComment().GetBody()

	if regAddLgtm.MatchString(comment) {
		return bot.addLGTM(cfg, e, log)
	}

	if regRemoveLgtm.MatchString(comment) {
		return bot.removeLGTM(cfg, e, log)
	}

	return nil
}

func (bot *robot) addLGTM(cfg *botConfig, e *sdk.IssueCommentEvent, log *logrus.Entry) error {
	org, repo := gc.GetOrgRepo(e.GetRepo())
	number := e.GetIssue().GetNumber()
	commenter := e.GetComment().GetUser().GetLogin()
	author := e.GetIssue().GetUser().GetLogin()

	pr := gc.PRInfo{Org: org, Repo: repo, Number: number}

	if author == commenter {
		return bot.cli.CreatePRComment(pr, commentAddLGTMBySelf)
	}

	v, err := bot.hasPermission(
		org, repo, commenter, cfg.CheckPermissionBasedOnSigOwners, e, cfg, log,
	)
	if err != nil {
		return err
	}
	if !v {
		return bot.cli.CreatePRComment(
			pr, fmt.Sprintf(commentNoPermissionForLgtmLabel, commenter),
		)
	}

	label := genLGTMLabel(commenter, cfg.LgtmCountsRequired)
	if label != lgtmLabel {
		if err := bot.createLabelIfNeed(org, repo, label); err != nil {
			log.WithError(err).Errorf("create repo label: %s", label)
		}
	}

	if err := bot.cli.AddPRLabel(pr, label); err != nil {
		return err
	}

	err = bot.cli.CreatePRComment(
		pr, fmt.Sprintf(commentAddLabel, label, commenter),
	)
	if err != nil {
		log.Error(err)
	}

	return bot.tryMerge(e, cfg, false, log)
}

func (bot *robot) removeLGTM(cfg *botConfig, e *sdk.IssueCommentEvent, log *logrus.Entry) error {
	org, repo := gc.GetOrgRepo(e.GetRepo())
	number := e.GetIssue().GetNumber()
	commenter := e.GetComment().GetUser().GetLogin()
	author := e.GetIssue().GetUser().GetLogin()
	pr := gc.PRInfo{Org: org, Repo: repo, Number: number}

	if commenter != author {
		v, err := bot.hasPermission(
			org, repo, commenter, cfg.CheckPermissionBasedOnSigOwners, e, cfg, log,
		)
		if err != nil {
			return err
		}
		if !v {
			return bot.cli.CreatePRComment(pr, fmt.Sprintf(
				commentNoPermissionForLabel, commenter, "remove", lgtmLabel,
			))
		}

		l := genLGTMLabel(commenter, cfg.LgtmCountsRequired)
		if err = bot.cli.RemovePRLabel(pr, l); err != nil {
			return err
		}

		return bot.cli.CreatePRComment(
			pr, fmt.Sprintf(commentRemovedLabel, l, commenter),
		)
	}

	// the author of pr can remove all of lgtm[-login name] kind labels
	lbs := sets.NewString()
	for _, l := range e.GetIssue().Labels {
		lbs.Insert(l.GetName())
	}
	if v := getLGTMLabelsOnPR(lbs); len(v) > 0 {
		for _, vv := range v {
			err := bot.cli.RemovePRLabel(pr, vv)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (bot *robot) createLabelIfNeed(org, repo, label string) error {
	p := gc.PRInfo{Org: org, Repo: repo}
	repoLabels, err := bot.cli.GetRepositoryLabels(p)
	if err != nil {
		return err
	}

	for _, v := range repoLabels {
		if v == label {
			return nil
		}
	}

	return bot.cli.CreateRepoLabel(org, repo, label)
}

func genLGTMLabel(commenter string, lgtmCount uint) string {
	if lgtmCount <= 1 {
		return lgtmLabel
	}

	l := fmt.Sprintf("%s-%s", lgtmLabel, strings.ToLower(commenter))
	if len(l) > labelLenLimit {
		return l[:labelLenLimit]
	}

	return l
}

func getLGTMLabelsOnPR(labels sets.String) []string {
	var r []string

	for l := range labels {
		if strings.HasPrefix(l, lgtmLabel) {
			r = append(r, l)
		}
	}

	return r
}

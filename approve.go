package main

import (
	"fmt"
	"regexp"

	gc "github.com/opensourceways/robot-github-lib/client"

	sdk "github.com/google/go-github/v36/github"
	"github.com/sirupsen/logrus"
)

const approvedLabel = "approved"

var (
	regAddApprove    = regexp.MustCompile(`(?mi)^/approve\s*$`)
	regRemoveApprove = regexp.MustCompile(`(?mi)^/approve cancel\s*$`)
)

func (bot *robot) handleApprove(e *sdk.IssueCommentEvent, cfg *botConfig, log *logrus.Entry) error {
	if !e.GetIssue().IsPullRequest() || !(e.GetIssue().GetState() == open) || !gc.IsCommentCreated(e) {
		return nil
	}

	comment := e.GetComment().GetBody()
	if regAddApprove.MatchString(comment) {
		return bot.AddApprove(cfg, e, log)
	}

	if regRemoveApprove.MatchString(comment) {
		return bot.removeApprove(cfg, e, log)
	}

	return nil
}

func (bot *robot) AddApprove(cfg *botConfig, e *sdk.IssueCommentEvent, log *logrus.Entry) error {
	org, repo := gc.GetOrgRepo(e.GetRepo())
	number := e.GetIssue().GetNumber()
	commenter := e.GetComment().GetUser().GetLogin()

	pr := gc.PRInfo{Org: org, Repo: repo, Number: number}

	v, err := bot.hasPermission(org, repo, commenter, false, e, cfg, log)
	if err != nil {
		return err
	}

	if !v {
		return bot.cli.CreatePRComment(pr, fmt.Sprintf(
			commentNoPermissionForLabel, commenter, "add", approvedLabel,
		))
	}

	if err := bot.cli.AddPRLabel(pr, approvedLabel); err != nil {
		return err
	}

	err = bot.cli.CreatePRComment(
		pr, fmt.Sprintf(commentAddLabel, approvedLabel, commenter),
	)
	if err != nil {
		log.Error(err)
	}

	return bot.tryMerge(e, cfg, false, log)
}

func (bot *robot) removeApprove(cfg *botConfig, e *sdk.IssueCommentEvent, log *logrus.Entry) error {
	org, repo := gc.GetOrgRepo(e.GetRepo())
	number := e.GetIssue().GetNumber()
	commenter := e.GetComment().GetUser().GetLogin()

	pr := gc.PRInfo{Org: org, Repo: repo, Number: number}

	v, err := bot.hasPermission(org, repo, commenter, false, e, cfg, log)
	if err != nil {
		return err
	}

	if !v {
		return bot.cli.CreatePRComment(pr, fmt.Sprintf(
			commentNoPermissionForLabel, commenter, "remove", approvedLabel,
		))
	}

	err = bot.cli.RemovePRLabel(pr, approvedLabel)
	if err != nil {
		return err
	}

	return bot.cli.CreatePRComment(
		pr, fmt.Sprintf(commentRemovedLabel, approvedLabel, commenter),
	)
}
